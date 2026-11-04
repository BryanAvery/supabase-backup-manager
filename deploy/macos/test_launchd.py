"""Safety tests for the M1-07 launchd generator (no root or Mac required)."""

from __future__ import annotations

import copy
import importlib.util
import json
import pathlib
import plistlib
import stat
import subprocess
import sys
import tempfile
import unittest

SCRIPT = pathlib.Path(__file__).with_name("launchd.py")
spec = importlib.util.spec_from_file_location("sbm_launchd", SCRIPT)
assert spec and spec.loader
service = importlib.util.module_from_spec(spec)
spec.loader.exec_module(service)

CONFIG = {
    "service_user": "_sbmbackup",
    "listen": "127.0.0.1:8765",
    "binary": str(service.BASE / "bin/backup-manager"),
    "catalogue": str(service.BASE / "private/catalogue.sqlite"),
    "tls_cert": str(service.BASE / "config/loopback.crt"),
    "tls_key": str(service.BASE / "private/loopback.key"),
}

AUTH = {
    "issuer": "https://id.example.invalid/",
    "audience": "api://backup-manager",
    "client_id": "synthetic-public-client",
    "required_acr": "urn:example:approved-mfa",
    "key_id": "synthetic-key-id",
    "public_key": str(service.BASE / "config/access-public.pem"),
}

BROWSER = {
    "authorise_url": "https://id.example.invalid/authorise",
    "token_url": "https://id.example.invalid/token",
    "redirect_uri": "https://127.0.0.1:8765/web/auth/callback",
    "api_scope": "api.read",
    "id_key_id": "synthetic-id-key",
    "id_public_key": str(service.BASE / "config/id-public.pem"),
}


class LaunchdTests(unittest.TestCase):
    def config(self):
        return copy.deepcopy(CONFIG)

    def test_locked_configuration_has_no_privileged_user_or_tokens(self):
        spec = service.render(self.config())
        self.assertEqual(spec["UserName"], "_sbmbackup")
        self.assertEqual(spec["Label"], service.LABEL)
        self.assertEqual(spec["ProgramArguments"][0], CONFIG["binary"])
        self.assertEqual(spec["ProgramArguments"][1:3], ["-listen", "127.0.0.1:8765"])
        self.assertEqual(spec["Umask"], 63)
        self.assertTrue(spec["RunAtLoad"])
        self.assertTrue(spec["KeepAlive"])
        self.assertGreaterEqual(spec["ThrottleInterval"], 10)
        self.assertEqual(spec["HardResourceLimits"]["Core"], 0)
        for disallowed in ("EnvironmentVariables", "SessionCreate", "RootDirectory", "Sockets",
                           "GroupName", "MachServices", "UserEnvironmentVariables"):
            self.assertNotIn(disallowed, spec)
        xml = plistlib.dumps(spec)
        self.assertEqual(plistlib.loads(xml), spec)
        joined = xml.lower()
        for bad in (b"client_secret", b"password", b"access_token", b"refresh_token"):
            self.assertNotIn(bad, joined)
        self.assertNotIn("-enable-bearer-auth", spec["ProgramArguments"])

    def test_signed_auth_and_browser_are_complete_and_explicit(self):
        c = self.config()
        c["auth"] = copy.deepcopy(AUTH)
        c["browser"] = copy.deepcopy(BROWSER)
        c["web_assets_dir"] = str(service.BASE / "web/dist")
        args = service.render(c)["ProgramArguments"]
        self.assertIn("-enable-bearer-auth", args)
        self.assertIn("-enable-browser-login", args)
        self.assertIn("-web-assets-dir", args)
        self.assertIn("urn:example:approved-mfa", args)
        self.assertNotIn("client_secret", " ".join(args).lower())
        self.assertIn(str(service.BASE / "config/id-public.pem"), args)

    def test_all_nonloopback_or_unsafe_ports_are_rejected(self):
        for listen in (
            "0.0.0.0:8765", "[::]:8765", "192.0.2.7:8765",
            "localhost:8765", "127.0.0.1:80", "[::1]:80",
            "127.0.0.1:0", "127.0.0.1:99999",
            "127.0.0.1:abc", "127.0.0.1:08765",
            "[::1]8765", "", "127.0.0.1:-1",
        ):
            with self.subTest(listen=listen):
                c = self.config()
                c["listen"] = listen
                with self.assertRaises(service.ConfigError):
                    service.render(c)
        c = self.config()
        c["listen"] = "[::1]:8765"
        self.assertEqual(service.render(c)["ProgramArguments"][2], "[::1]:8765")

    def test_refuse_unsupported_users_and_path_substitution(self):
        for user in ("root", "admin", "_root", "_sbmbackup;cat", "_", "_sbm backup", None):
            c = self.config()
            c["service_user"] = user
            with self.subTest(user=user), self.assertRaises(service.ConfigError):
                service.render(c)
        for path in (
            "/tmp/backup-manager", "/Library/Application Support/SupabaseBackupManager/bin/../../tmp/bad",
            "bin/backup-manager", "/Library/Application Support/SupabaseBackupManager/private/other.sqlite",
        ):
            c = self.config()
            c["binary"] = path
            with self.subTest(path=path), self.assertRaises(service.ConfigError):
                service.render(c)
        c = self.config()
        c["auth"] = {"issuer": AUTH["issuer"]}
        with self.assertRaises(service.ConfigError):
            service.render(c)
        c = self.config()
        c["browser"] = copy.deepcopy(BROWSER)
        with self.assertRaises(service.ConfigError):
            service.render(c)
        c = self.config()
        c["web_assets_dir"] = str(service.BASE / "web/dist")
        with self.assertRaises(service.ConfigError):
            service.render(c)

    def test_browser_rejects_wrong_redirect_or_public_key(self):
        c = self.config()
        c["auth"] = copy.deepcopy(AUTH)
        c["browser"] = copy.deepcopy(BROWSER)
        c["browser"]["redirect_uri"] = "https://localhost:8765/web/auth/callback"
        with self.assertRaises(service.ConfigError):
            service.render(c)
        c["browser"]["redirect_uri"] = BROWSER["redirect_uri"]
        c["browser"]["id_public_key"] = "/tmp/attacker.pem"
        with self.assertRaises(service.ConfigError):
            service.render(c)

    def test_staging_refuses_world_readable_config_symlink_and_overwrite(self):
        with tempfile.TemporaryDirectory() as temp:
            root = pathlib.Path(temp)
            config = root / "config.json"
            config.write_text(json.dumps(CONFIG))
            config.chmod(0o600)
            loaded = service.load_config(config)
            self.assertEqual(loaded, CONFIG)
            rendered = root / "staged.plist"
            success = subprocess.run(
                [sys.executable, str(SCRIPT), "render", "--config", str(config),
                 "--output", str(rendered)],
                capture_output=True, text=True,
            )
            self.assertEqual(success.returncode, 0, success.stderr)
            self.assertEqual(stat.S_IMODE(rendered.stat().st_mode), 0o600)
            parsed = plistlib.loads(rendered.read_bytes())
            self.assertEqual(parsed["UserName"], "_sbmbackup")
            second = subprocess.run(
                [sys.executable, str(SCRIPT), "render", "--config", str(config),
                 "--output", str(rendered)],
                capture_output=True, text=True,
            )
            self.assertEqual(second.returncode, 2, second.stdout)
            config.chmod(0o644)
            with self.assertRaises(service.ConfigError):
                service.load_config(config)
            config.chmod(0o600)
            linked = root / "linked.json"
            linked.symlink_to(config)
            with self.assertRaises(service.ConfigError):
                service.load_config(linked)
            config.write_text(json.dumps({**CONFIG, "client_secret": "should-not-be-here"}))
            with self.assertRaises(service.ConfigError):
                service.load_config(config)
            config.write_text(json.dumps(CONFIG))
            config.chmod(0o600)
            no_output = subprocess.run(
                [sys.executable, str(SCRIPT), "render", "--config", str(config),
                 "--output", str(service.PLIST)],
                capture_output=True, text=True,
            )
            self.assertEqual(no_output.returncode, 2)
            self.assertFalse(service.PLIST.exists() and service.PLIST.is_symlink())

    def test_root_or_unknown_field_never_becomes_launchd_argument(self):
        for bad in ("client_secret", "password", "token", "environment"):
            c = self.config()
            c[bad] = "unsafe"
            with self.subTest(field=bad), self.assertRaises(service.ConfigError):
                service.render(c)


if __name__ == "__main__":
    unittest.main()

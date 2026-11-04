#!/usr/bin/env python3
"""Render and audit a least-privileged macOS launchd system daemon.

Rendering makes no system changes. This script never installs a service,
creates users, launches a process, downloads files, or accepts credentials.
The operator performs the reviewed, explicit launchctl steps in the runbook.
"""

from __future__ import annotations

import argparse
import grp
import ipaddress
import json
import os
import pathlib
import plistlib
import pwd
import re
import stat
import sys

LABEL = "org.example.supabasebackupmanager.daemon"
BASE = pathlib.Path("/Library/Application Support/SupabaseBackupManager")
PLIST = pathlib.Path("/Library/LaunchDaemons") / (LABEL + ".plist")
MAX_CONFIG_BYTES = 16384
AUTH_KEYS = {
    "issuer": "-auth-issuer",
    "audience": "-auth-audience",
    "client_id": "-auth-client-id",
    "required_acr": "-auth-required-acr",
    "key_id": "-auth-key-id",
    "public_key": "-auth-public-key",
}
BROWSER_KEYS = {
    "authorise_url": "-oidc-authorize-url",
    "token_url": "-oidc-token-url",
    "redirect_uri": "-oidc-redirect-uri",
    "api_scope": "-oidc-api-scope",
    "id_key_id": "-oidc-id-key-id",
    "id_public_key": "-oidc-id-public-key",
}
CONFIG_KEYS = {
    "service_user", "listen", "binary", "catalogue", "tls_cert", "tls_key",
    "auth", "browser", "web_assets_dir",
}
SAFE_USER = re.compile(r"^_[a-z][a-z0-9_]{1,28}$")
SAFE_PUBLIC = re.compile(r"^[^\x00-\x1f\x7f]{1,512}$")


class ConfigError(ValueError):
    """Refuse unsafe settings without partially installing anything."""


def secure_path(value: object, root: pathlib.Path = BASE) -> pathlib.Path:
    if not isinstance(value, str) or not value or "\x00" in value:
        raise ConfigError("absolute service path required")
    p = pathlib.Path(value)
    if not p.is_absolute() or ".." in p.parts or pathlib.PurePath(value) != p:
        raise ConfigError("non-canonical or relative path rejected")
    if p != root and root not in p.parents:
        raise ConfigError("service assets must remain inside the fixed application root")
    return p


def public(value: object, label: str) -> str:
    if not isinstance(value, str) or not SAFE_PUBLIC.fullmatch(value) or value.strip() != value:
        raise ConfigError(f"invalid or missing public setting: {label}")
    if value.startswith("-"):
        raise ConfigError(f"public setting cannot be parsed as a flag: {label}")
    return value


def load_config(path: pathlib.Path) -> dict:
    if not path.is_absolute():
        raise ConfigError("configuration path must be absolute")
    info = path.lstat()
    if not stat.S_ISREG(info.st_mode) or info.st_mode & 0o077:
        raise ConfigError("deployment configuration must be a private regular file (mode 0600)")
    if info.st_size <= 0 or info.st_size > MAX_CONFIG_BYTES:
        raise ConfigError("deployment configuration exceeds size bound")
    raw = path.read_bytes()
    if len(raw) > MAX_CONFIG_BYTES:
        raise ConfigError("deployment configuration exceeds size bound")
    try:
        config = json.loads(raw)
    except (UnicodeDecodeError, json.JSONDecodeError) as e:
        raise ConfigError("invalid deployment JSON") from e
    if not isinstance(config, dict) or set(config) - CONFIG_KEYS:
        raise ConfigError("unsupported deployment configuration keys")
    # Prevent insertion of tokens and secrets via unexpected config fields.
    if any(word in raw.lower() for word in (b"client_secret", b"access_token", b"refresh_token", b"password")):
        raise ConfigError("credentials must never be passed through launchd arguments")
    return config


def validate_listen(value: object) -> str:
    listen = public(value, "listen")
    if listen.startswith("["):
        try:
            host, port = listen[1:].split("]:", 1)
        except ValueError as e:
            raise ConfigError("malformed IPv6 loopback listener") from e
    else:
        try:
            host, port = listen.rsplit(":", 1)
        except ValueError as e:
            raise ConfigError("malformed IPv4 loopback listener") from e
    try:
        ip = ipaddress.ip_address(host)
        port_num = int(port)
    except ValueError as e:
        raise ConfigError("listener must be numeric loopback IP and port") from e
    if not ip.is_loopback or not (1024 <= port_num <= 65535) or str(port_num) != port:
        raise ConfigError("listener must use unprivileged numeric loopback port")
    return listen


def render(config: dict) -> dict:
    if not isinstance(config, dict) or set(config) - CONFIG_KEYS:
        raise ConfigError("unsupported configuration")
    user = public(config.get("service_user"), "service_user")
    if not SAFE_USER.fullmatch(user):
        raise ConfigError("use a dedicated underscore-prefixed non-root service identity")
    listen = validate_listen(config.get("listen"))
    binary = secure_path(config.get("binary"))
    catalogue = secure_path(config.get("catalogue"))
    cert = secure_path(config.get("tls_cert"))
    key = secure_path(config.get("tls_key"))
    if binary != BASE / "bin/backup-manager":
        raise ConfigError("binary must use root-owned fixed installation path")
    if catalogue != BASE / "private/catalogue.sqlite":
        raise ConfigError("catalogue must use the private service path")
    if cert != BASE / "config/loopback.crt" or key != BASE / "private/loopback.key":
        raise ConfigError("TLS certificate/key must use fixed service paths")
    args = [
        str(binary),
        "-listen", listen,
        "-catalogue", str(catalogue),
        "-tls-cert", str(cert),
        "-tls-key", str(key),
    ]
    auth = config.get("auth")
    browser = config.get("browser")
    if auth is not None:
        if not isinstance(auth, dict) or set(auth) != set(AUTH_KEYS):
            raise ConfigError("partial or unknown signed JWT verifier configuration")
        args.append("-enable-bearer-auth")
        for name, flag in AUTH_KEYS.items():
            value = public(auth[name], name)
            if name == "public_key":
                value = str(secure_path(value))
                if value != str(BASE / "config/access-public.pem"):
                    raise ConfigError("access public key must be in trusted config directory")
            args.extend((flag, value))
    if browser is not None:
        if auth is None or not isinstance(browser, dict) or set(browser) != set(BROWSER_KEYS):
            raise ConfigError("OIDC browser requires complete bearer verifier configuration")
        args.append("-enable-browser-login")
        for name, flag in BROWSER_KEYS.items():
            value = public(browser[name], name)
            if name == "id_public_key":
                value = str(secure_path(value))
                if value != str(BASE / "config/id-public.pem"):
                    raise ConfigError("ID public key must be in trusted config directory")
            args.extend((flag, value))
        expected_redirect = f"https://{listen}/web/auth/callback"
        if browser["redirect_uri"] != expected_redirect:
            raise ConfigError("browser redirect must match exact HTTPS loopback host and port")
    assets = config.get("web_assets_dir")
    if assets is not None:
        if browser is None or str(secure_path(assets)) != str(BASE / "web/dist"):
            raise ConfigError("dashboard requires secure OIDC browser and fixed asset directory")
        args.extend(("-web-assets-dir", assets))

    return {
        "Label": LABEL,
        "ProgramArguments": args,
        "UserName": user,
        "RunAtLoad": True,
        "KeepAlive": True,
        "ThrottleInterval": 15,
        "ExitTimeOut": 10,
        "ProcessType": "Background",
        "Umask": 63,  # decimal for POSIX 077
        "HardResourceLimits": {"Core": 0, "NumberOfFiles": 256},
        "StandardOutPath": str(BASE / "logs/stdout.log"),
        "StandardErrorPath": str(BASE / "logs/stderr.log"),
    }


def check_regular(path: pathlib.Path, owner_uid: int, writable_by_other: bool = False) -> None:
    info = path.lstat()
    if not stat.S_ISREG(info.st_mode) or info.st_uid != owner_uid:
        raise ConfigError(f"invalid owner or type for {path}")
    if info.st_mode & (0o022 if not writable_by_other else 0o002):
        raise ConfigError(f"untrusted write permission for {path}")


def audit(config: dict, spec: dict) -> None:
    """Audit existing operator-provisioned paths without modifying them."""
    if sys.platform != "darwin":
        raise ConfigError("deployment permission audit requires macOS")
    try:
        user = pwd.getpwnam(spec["UserName"])
    except KeyError as e:
        raise ConfigError("dedicated service account has not been provisioned") from e
    if user.pw_uid == 0 or not user.pw_shell.endswith(("/false", "/nologin")):
        raise ConfigError("service user must be non-root with a non-interactive login shell")
    if user.pw_uid in (0, 1):
        raise ConfigError("unsafe service account UID")
    try:
        admin = grp.getgrnam("admin")
    except KeyError as e:
        raise ConfigError("could not independently verify admin group membership") from e
    if user.pw_gid == admin.gr_gid or user.pw_name in admin.gr_mem:
        raise ConfigError("service user must not be in the macOS admin group")

    for folder, uid, perms in [
        (BASE, 0, 0o755),
        (BASE / "bin", 0, 0o755),
        (BASE / "config", 0, 0o755),
        (BASE / "private", user.pw_uid, 0o700),
        (BASE / "logs", user.pw_uid, 0o700),
    ]:
        info = folder.lstat()
        if not stat.S_ISDIR(info.st_mode) or info.st_uid != uid or stat.S_IMODE(info.st_mode) != perms:
            raise ConfigError(f"invalid service directory owner or permissions: {folder}")
    bin_path = BASE / "bin/backup-manager"
    check_regular(bin_path, 0)
    if not os.access(bin_path, os.X_OK):
        raise ConfigError("binary is not executable")
    cert = BASE / "config/loopback.crt"
    check_regular(cert, 0)
    key = BASE / "private/loopback.key"
    key_info = key.lstat()
    if not stat.S_ISREG(key_info.st_mode) or key_info.st_uid != user.pw_uid or stat.S_IMODE(key_info.st_mode) != 0o600:
        raise ConfigError("TLS private key must be mode 0600 and owned by the service account")
    if config.get("auth"):
        check_regular(BASE / "config/access-public.pem", 0)
    if config.get("browser"):
        check_regular(BASE / "config/id-public.pem", 0)
    if config.get("web_assets_dir"):
        # The Go service independently validates the production asset bundle.
        info = (BASE / "web/dist").lstat()
        if not stat.S_ISDIR(info.st_mode) or info.st_uid != 0 or info.st_mode & 0o022:
            raise ConfigError("browser assets must live in a root-owned read-only directory")


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("command", choices=("render", "audit"))
    parser.add_argument("--config", required=True, type=pathlib.Path)
    parser.add_argument("--output", type=pathlib.Path)
    options = parser.parse_args()

    try:
        config = load_config(options.config)
        spec = render(config)
        if options.command == "audit":
            if options.output is not None:
                raise ConfigError("--output only valid when rendering")
            audit(config, spec)
            print("Mac deployment files and service identity passed preflight audit")
            return 0
        if options.output is None or not options.output.is_absolute() or options.output == PLIST:
            raise ConfigError("render requires a staging path; refuse direct system plist writes")
        output = options.output
        if output.is_symlink() or output.exists():
            raise ConfigError("refuse to overwrite an existing file or symlink")
        payload = plistlib.dumps(spec, fmt=plistlib.FMT_XML, sort_keys=False)
        fd = os.open(output, os.O_WRONLY | os.O_CREAT | os.O_EXCL, 0o600)
        try:
            with os.fdopen(fd, "wb") as stream:
                stream.write(payload)
        except BaseException:
            output.unlink(missing_ok=True)
            raise
        print(f"Rendered staged launchd configuration: {output}")
        return 0
    except (ConfigError, OSError) as err:
        print(f"Refused deployment: {err}", file=sys.stderr)
        return 2


if __name__ == "__main__":
    sys.exit(main())

#!/usr/bin/env python3
"""Synthetic M1-07 headless operational smoke check.

Runs the real Go daemon directly as a non-root process with a disposable
SQLite catalogue and one-day, localhost-only synthetic TLS certificate.
It does not install launchd, access Keychain, contact an IdP/Supabase, or
simulate real GUI logout/reboot. A passing test is NOT M1-A08/M1-A10 proof.
"""

from __future__ import annotations

import argparse
import json
import os
import pathlib
import signal
import socket
import ssl
import stat
import subprocess
import sys
import tempfile
import time
import urllib.error
import urllib.request


def available_port() -> int:
    with socket.socket(socket.AF_INET, socket.SOCK_STREAM) as listener:
        listener.bind(("127.0.0.1", 0))
        return listener.getsockname()[1]


def synthetic_certificate(root: pathlib.Path) -> tuple[pathlib.Path, pathlib.Path]:
    cert = root / "loopback.crt"
    key = root / "loopback.key"
    openssl_conf = root / "openssl.cnf"
    openssl_conf.write_text(
        "[req]\n"
        "distinguished_name=dn\n"
        "x509_extensions=v3_req\n"
        "prompt=no\n"
        "[dn]\n"
        "CN=synthetic-local-test\n"
        "[v3_req]\n"
        "basicConstraints=critical,CA:FALSE\n"
        "keyUsage=critical,digitalSignature,keyEncipherment\n"
        "extendedKeyUsage=serverAuth\n"
        "subjectAltName=IP:127.0.0.1\n"
    )
    subprocess.run(
        ["openssl", "req", "-x509", "-newkey", "rsa:2048", "-nodes",
         "-days", "1", "-keyout", str(key), "-out", str(cert),
         "-config", str(openssl_conf), "-extensions", "v3_req"],
        check=True, stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL,
        timeout=15,
    )
    key.chmod(0o600)
    if stat.S_IMODE(key.stat().st_mode) != 0o600:
        raise AssertionError("synthetic key unexpectedly public")
    return cert, key


def check_https(origin: str, cert: pathlib.Path, path: str, expected: int) -> dict | None:
    # Explicitly trust only the ephemeral test certificate. Do not disable TLS
    # verification, hostname checks or the TLS 1.3 minimum.
    context = ssl.create_default_context(cafile=str(cert))
    context.minimum_version = ssl.TLSVersion.TLSv1_3
    request = urllib.request.Request(origin + path, headers={"Accept": "application/json"})
    try:
        with urllib.request.urlopen(request, context=context, timeout=2) as response:
            status = response.status
            body = response.read(16384)
    except urllib.error.HTTPError as error:
        status = error.code
        body = error.read(16384)
    if status != expected:
        raise AssertionError(f"unexpected {path} status: {status}, expected {expected}")
    return json.loads(body) if status == 200 else None


def verify_local_only(port: int) -> None:
    with socket.socket(socket.AF_INET, socket.SOCK_STREAM) as candidate:
        candidate.settimeout(0.5)
        try:
            candidate.connect(("127.0.0.2", port))
        except OSError:
            return  # listener is pinned to exact loopback IP, not entire subnet
        raise AssertionError("service accepted another loopback listener address")


def wait_ready(process: subprocess.Popen, origin: str, cert: pathlib.Path) -> None:
    deadline = time.monotonic() + 15
    while time.monotonic() < deadline:
        if process.poll() is not None:
            raise AssertionError(f"service exited before becoming healthy ({process.returncode})")
        try:
            info = check_https(origin, cert, "/api/v1/health", 200)
            if info and info.get("status") == "ok" and info.get("version"):
                return
        except (OSError, ssl.SSLError, urllib.error.URLError, ValueError):
            pass
        time.sleep(0.1)
    raise AssertionError("service failed TLS health-check startup bound")


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--binary", required=True, type=pathlib.Path)
    args = parser.parse_args()

    if os.geteuid() == 0:
        print("Refusing headless smoke test as root", file=sys.stderr)
        return 2
    if not args.binary.is_absolute() or not args.binary.is_file():
        print("An absolute compiled Go binary must be provided", file=sys.stderr)
        return 2
    with tempfile.TemporaryDirectory(prefix="sbm-m1-headless-") as tmp:
        root = pathlib.Path(tmp)
        root.chmod(0o700)
        cert, key = synthetic_certificate(root)
        catalogue = root / "catalogue.sqlite"
        port = available_port()
        origin = f"https://127.0.0.1:{port}"
        args_for_daemon = [
            str(args.binary),
            "-listen", f"127.0.0.1:{port}",
            "-catalogue", str(catalogue),
            "-tls-cert", str(cert),
            "-tls-key", str(key),
        ]
        # Remove GUI/session variables to exercise the noninteractive daemon
        # path. This does not prove real post-logout or reboot behaviour.
        env = {
            name: value for name, value in os.environ.items()
            if name not in {"DISPLAY", "SSH_AUTH_SOCK", "LAUNCHD_SOCKET",
                            "XPC_SERVICE_NAME", "XPC_FLAGS"}
        }
        env["HOME"] = str(root / "absent-user-home")
        devnull = subprocess.DEVNULL

        def start() -> subprocess.Popen:
            return subprocess.Popen(
                args_for_daemon, cwd=root, env=env,
                stdin=devnull, stdout=devnull, stderr=devnull,
                close_fds=True, start_new_session=True,
            )

        process = start()
        try:
            wait_ready(process, origin, cert)
            assert check_https(origin, cert, "/api/v1/projects", 401) is None
            verify_local_only(port)
            if not catalogue.is_file() or stat.S_IMODE(catalogue.stat().st_mode) != 0o600:
                raise AssertionError("private SQLite catalogue was not created safely")
            print("PASS: TLS 1.3 verified health, deny-all projects, private SQLite, exact loopback")

            # Kill without graceful shutdown, then recover SQLite on a new
            # daemon process. This models a crash, not launchd supervision.
            process.kill()
            process.wait(timeout=5)
            process = start()
            wait_ready(process, origin, cert)
            assert check_https(origin, cert, "/api/v1/projects", 401) is None
            print("PASS: cold restart after crash recovered private catalogue")

            process.send_signal(signal.SIGTERM)
            status = process.wait(timeout=8)
            if status != 0:
                raise AssertionError(f"SIGTERM did not cause clean exit: {status}")
            print("PASS: SIGTERM clean shutdown with no graphical session")

            # Repeat startup after clean shutdown to prove catalogue remains
            # usable across successive headless restarts.
            process = start()
            wait_ready(process, origin, cert)
            assert check_https(origin, cert, "/api/v1/projects", 401) is None
            print("PASS: clean restart preserved default-deny access")
            return 0
        finally:
            if process.poll() is None:
                process.kill()
            process.wait(timeout=5)


if __name__ == "__main__":
    try:
        sys.exit(main())
    except (AssertionError, OSError, subprocess.SubprocessError, ValueError) as exc:
        print(f"FAIL: synthetic daemon smoke check: {exc}", file=sys.stderr)
        sys.exit(1)

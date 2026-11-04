#!/usr/bin/env python3
"""Bootstrap only the dedicated M0 source fixture, never application schemas."""
import os
from pathlib import Path
import subprocess
import sys
from urllib.parse import urlsplit, unquote

MARKER = "m0-source-fixture-v1"

def run(url, *args):
    p = subprocess.run(["psql", "-X", "-q", "-A", "-t", "-v", "ON_ERROR_STOP=1",
                        "-d", url, *args], capture_output=True, text=True, timeout=45)
    if p.returncode:
        raise RuntimeError("Fixture verification/setup failed; database output withheld")
    return p.stdout.strip()

def project(url):
    p = urlsplit(url)
    username = unquote(p.username or "")
    if (p.scheme not in ("postgres", "postgresql")
            or not (p.hostname or "").lower().endswith(".pooler.supabase.com")
            or not username.startswith("postgres.")
            or not p.password):
        raise ValueError("Only explicit Supabase pooler project connections are supported")
    ref = username.removeprefix("postgres.")
    if len(ref) != 20 or not ref.isalnum() or not ref.islower():
        raise ValueError("Invalid Supabase project reference")
    return ref

def ensure_fixture(source, target):
    if project(source) == project(target):
        raise ValueError("Source and recovery project references match")
    # Ensure both routes connect, before writing to either (writes are source-only).
    run(source, "-c", "SELECT 1")
    run(target, "-c", "SELECT 1")
    marker = run(source, "-c", """SELECT coalesce(
        (SELECT coalesce(obj_description(oid, 'pg_namespace'), '<unmarked>')
         FROM pg_namespace WHERE nspname='m0_fixture'), '<absent>')""")
    if marker == MARKER:
        # Refuse to accept a tampered fixture without at least its expected baseline.
        count = run(source, "-c", "SELECT count(*) FROM m0_fixture.parent")
        if count != "2":
            raise RuntimeError("Existing managed fixture has unexpected baseline")
        return "Existing managed source fixture verified"
    if marker != "<absent>":
        raise RuntimeError("Source m0_fixture already exists but is not managed; refusing changes")
    run(source, "-f", str(Path(__file__).with_name("source_fixture.sql")))
    return "Created dedicated source fixture"

def main():
    source, target = os.getenv("M0_SOURCE_URL"), os.getenv("M0_TARGET_URL")
    if not source or not target:
        print("ERROR: Both database connection secrets are required")
        return 2
    try:
        print(ensure_fixture(source, target))
        return 0
    except (ValueError, RuntimeError, subprocess.TimeoutExpired) as exc:
        print("ERROR: " + str(exc))
        return 1

if __name__ == "__main__":
    sys.exit(main())

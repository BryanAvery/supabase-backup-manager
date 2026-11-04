#!/usr/bin/env python3
"""Read-only diagnostics for M0 connection identity. Never restores or drops data."""
import json
import os
import subprocess
import sys
from urllib.parse import unquote, urlsplit

QUERY = """SELECT json_build_object(
 'database', current_database(),
 'database_oid', (SELECT oid FROM pg_database WHERE datname=current_database()),
 'server_address', inet_server_addr()::text,
 'server_port', inet_server_port(),
 'role', current_user)::text"""
CLUSTER_QUERY = "SELECT system_identifier::text FROM pg_control_system()"

def inspect(url):
    parsed = urlsplit(url)
    if parsed.scheme not in ("postgres", "postgresql") or not parsed.hostname:
        raise ValueError("Invalid PostgreSQL URL")
    username = unquote(parsed.username or "")
    host = parsed.hostname.lower()
    project = username[9:] if username.startswith("postgres.") and host.endswith(".pooler.supabase.com") else None
    if project and not (len(project) == 20 and project.isalnum() and project.islower()):
        project = None
    def query(statement):
        # URL is supplied by environment, not printed. No SQL writes.
        proc = subprocess.run(["psql", "-X", "-q", "-A", "-t", "-v", "ON_ERROR_STOP=1", "-d", url, "-c", statement],
                              capture_output=True, text=True, timeout=20)
        if proc.returncode:
            raise RuntimeError("Database read failed (connection details withheld)")
        return proc.stdout.strip()
    data = json.loads(query(QUERY))
    try:
        data["cluster_id"] = query(CLUSTER_QUERY)
    except (RuntimeError, subprocess.TimeoutExpired):
        data["cluster_id"] = None
    data["project"] = project
    data["is_supabase_pooler"] = host.endswith(".pooler.supabase.com")
    return data

def main():
    source = os.environ.get("M0_SOURCE_URL")
    target = os.environ.get("M0_TARGET_URL")
    if not source or not target:
        print("ERROR: Both M0_SOURCE_URL and M0_TARGET_URL are required")
        return 2
    try:
        a, b = inspect(source), inspect(target)
    except (ValueError, RuntimeError, subprocess.TimeoutExpired) as exc:
        print("ERROR: " + str(exc))
        return 2
    print("SOURCE and TARGET: connected successfully")
    print("Database names match:", a["database"] == b["database"])
    print("Database OIDs match:", a["database_oid"] == b["database_oid"])
    print("Backend addresses match:", a["server_address"] == b["server_address"])
    print("Supabase pooler routing references present:", bool(a["project"] and b["project"]))
    print("Supabase project references match:", a["project"] == b["project"] if a["project"] and b["project"] else "not comparable")
    print("PostgreSQL cluster identifiers available:", bool(a["cluster_id"] and b["cluster_id"]))
    if a["cluster_id"] and b["cluster_id"]:
        print("PostgreSQL cluster identifiers match:", a["cluster_id"] == b["cluster_id"])
    if a["project"] and b["project"] and a["project"] == b["project"]:
        print("UNSAFE: Both Supabase pooler URLs route to the same project.")
        return 1
    if a["cluster_id"] and b["cluster_id"] and a["cluster_id"] == b["cluster_id"]:
        print("REVIEW: Cluster identifiers match. This can occur with cloned clusters;")
        print("it does not by itself establish whether the Supabase projects are the same.")
        return 0
    if a["project"] and b["project"] and a["project"] != b["project"]:
        print("REVIEW: Different Supabase project routes; confirm target is disposable.")
    else:
        print("REVIEW: Project routing cannot be compared; check both URLs manually.")
    print("Diagnostic only: no M0 safety checks have been bypassed.")
    return 0

if __name__ == "__main__":
    sys.exit(main())

#!/usr/bin/env python3
"""M0 PostgreSQL backup/restore validation. Requires pg_dump, pg_restore and psql."""
import argparse
import datetime as dt
import json
import os
from pathlib import Path
import subprocess
import sys
import tempfile
from urllib.parse import urlsplit, unquote

def run(args, env, *, capture=True):
    p = subprocess.run(args, env=env, text=True, stdout=subprocess.PIPE if capture else None, stderr=subprocess.PIPE)
    if p.returncode:
        raise RuntimeError(f"{args[0]} failed ({p.returncode}): {p.stderr[-1500:] if p.stderr else ''}")
    return p.stdout.strip() if capture else ""

def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--source-url", default=os.getenv("M0_SOURCE_URL"))
    ap.add_argument("--target-url", default=os.getenv("M0_TARGET_URL"))
    ap.add_argument("--schema", default="m0_fixture")
    ap.add_argument("--output", default="reports")
    ap.add_argument("--confirm-isolated-target", action="store_true")
    ap.add_argument("--reset-managed-fixture", action="store_true")
    args = ap.parse_args()
    if not args.source_url or not args.target_url or not args.confirm_isolated_target:
        ap.error("Provide M0_SOURCE_URL, M0_TARGET_URL and --confirm-isolated-target")
    if args.source_url == args.target_url:
        ap.error("Source and target connection URLs must differ")
    if not args.schema.replace("_", "").isalnum() or not args.schema[0].isalpha():
        ap.error("Invalid schema identifier")
    stamp = dt.datetime.now(dt.timezone.utc).strftime("%Y%m%dT%H%M%SZ")
    out = Path(args.output)
    out.mkdir(parents=True, exist_ok=True)
    report = {"run": stamp, "schema": args.schema, "checks": [], "status": "FAIL"}
    def check(name, fn):
        try:
            detail = fn()
            report["checks"].append({"name": name, "status": "PASS", "detail": str(detail)[:500]})
        except Exception as e:
            report["checks"].append({"name": name, "status": "FAIL", "detail": str(e)[:500]})
            raise
    def sql(url, query):
        return run(["psql", "-X", "-v", "ON_ERROR_STOP=1", "-At", "-d", url, "-c", query], os.environ.copy())
    def fingerprint(url):
        # Deterministic schema structure fingerprint (does not include table data).
        q = f"""SELECT md5(coalesce(string_agg(table_name || ':' || column_name || ':' || data_type || ':' || is_nullable, '|' ORDER BY table_name, ordinal_position), '')) FROM information_schema.columns WHERE table_schema = '{args.schema}'"""
        return sql(url, q)
    try:
        check("distinct database identities", lambda: verify_distinct(args.source_url, args.target_url, sql))
        check("source schema exists", lambda: assert_schema(args.source_url, args.schema, sql))
        if args.reset_managed_fixture:
            check("reset managed target fixture", lambda: reset_managed(args.target_url, args.schema, sql))
        check("target schema absent", lambda: assert_absent(args.target_url, args.schema, sql))
        with tempfile.TemporaryDirectory(prefix="m0-") as tmp:
            dump = str(Path(tmp) / "fixture.dump")
            check("backup", lambda: run(["pg_dump", "-Fc", "-n", args.schema, "-f", dump, "-d", args.source_url], os.environ.copy()))
            check("archive readable", lambda: run(["pg_restore", "--list", dump], os.environ.copy())[:100])
            check("restore", lambda: run(["pg_restore", "--exit-on-error", "--no-owner", "-d", args.target_url, dump], os.environ.copy()))
            check("mark managed fixture", lambda: sql(args.target_url, "COMMENT ON SCHEMA " + args.schema + " IS 'm0-runner-managed-v1'"))
            check("schema fingerprint", lambda: compare(fingerprint(args.source_url), fingerprint(args.target_url)))
            check("table row counts", lambda: compare(table_counts(args.source_url, args.schema, sql), table_counts(args.target_url, args.schema, sql)))
            check("RLS flags", lambda: compare(rls_flags(args.source_url, args.schema, sql), rls_flags(args.target_url, args.schema, sql)))
            check("RLS policies", lambda: compare(policies(args.source_url, args.schema, sql), policies(args.target_url, args.schema, sql)))
        report["status"] = "PASS"
    except Exception as e:
        report["error"] = str(e)[:500]
    finally:
        path = out / f"m0-{stamp}.json"
        path.write_text(json.dumps(report, indent=2) + "\n")
        print(f"{report['status']}: {path}")
    return 0 if report["status"] == "PASS" else 1

def verify_distinct(source, target, sql):
    """Fail closed if the two connections cannot be independently distinguished."""
    statement = ("SELECT system_identifier::text || ':' || "
                 "(SELECT oid::text FROM pg_database WHERE datname=current_database()) "
                 "FROM pg_control_system()")
    try:
        source_id = sql(source, statement)
        target_id = sql(target, statement)
    except RuntimeError:
        # Hosted Supabase often restricts pg_control_system(). The pooler
        # routes postgres.<project_ref> usernames to a specific project.
        # Only accept this fallback when both URLs are Supabase pooler
        # connections and contain different, valid project references.
        def pooler_project(url):
            parsed = urlsplit(url)
            host = (parsed.hostname or "").lower()
            username = unquote(parsed.username or "")
            if (parsed.scheme not in ("postgres", "postgresql")
                    or not host.endswith(".pooler.supabase.com")
                    or not username.startswith("postgres.")
                    or not parsed.password
                    or not parsed.path.strip("/")):
                raise AssertionError(
                    "Database identity could not be verified. Use two distinct "
                    "Supabase pooler URLs (postgres.<project_ref> usernames), "
                    "or grant approved access to pg_control_system().")
            project = username.removeprefix("postgres.")
            if not (len(project) == 20 and project.isalnum() and project.islower()):
                raise AssertionError("Invalid Supabase pooler project reference")
            return project
        source_project = pooler_project(source)
        target_project = pooler_project(target)
        if source_project == target_project:
            raise AssertionError("Source and target Supabase project references match")
        # Do not print project refs or connection details into public logs.
        return "Distinct Supabase pooler project references"
    if not source_id or not target_id or source_id == target_id:
        raise AssertionError("Source and recovery resolve to the same database identity")
    return "Verified distinct PostgreSQL databases"

def compare(a, b):
    if a != b:
        raise AssertionError(f"Mismatch: source={str(a)[:180]}, target={str(b)[:180]}")
    return "Matched"

def assert_schema(url, schema, sql):
    if sql(url, f"SELECT count(*) FROM pg_namespace WHERE nspname='{schema}'") != "1":
        raise AssertionError("Source fixture schema missing")
    return "Present"

def reset_managed(url, schema, sql):
    marker = sql(url, f"SELECT coalesce(obj_description(oid, 'pg_namespace'),'') FROM pg_namespace WHERE nspname='{schema}'")
    if not marker:
        return "No existing fixture"
    if marker != "m0-runner-managed-v1":
        raise AssertionError("Existing target fixture is not runner-managed; refusing cleanup")
    sql(url, f'DROP SCHEMA "{schema}" CASCADE')
    return "Removed managed fixture"

def assert_absent(url, schema, sql):
    if sql(url, f"SELECT count(*) FROM pg_namespace WHERE nspname='{schema}'") != "0":
        raise AssertionError("Target schema already exists; refusing to overwrite")
    return "Absent"

def table_counts(url, schema, sql):
    tables = sql(url, f"SELECT tablename FROM pg_tables WHERE schemaname='{schema}' ORDER BY tablename").splitlines()
    return [(t, sql(url, f'SELECT count(*) FROM "{schema}"."{t}"')) for t in tables]

def rls_flags(url, schema, sql):
    return sql(url, f"SELECT relname || ':' || relrowsecurity || ':' || relforcerowsecurity FROM pg_class c JOIN pg_namespace n ON n.oid=c.relnamespace WHERE n.nspname='{schema}' AND c.relkind IN ('r','p') ORDER BY relname")

def policies(url, schema, sql):
    return sql(url, f"SELECT policyname || ':' || permissive || ':' || roles::text || ':' || cmd || ':' || coalesce(qual,'') || ':' || coalesce(with_check,'') FROM pg_policies WHERE schemaname='{schema}' ORDER BY tablename,policyname")

if __name__ == "__main__":
    sys.exit(main())

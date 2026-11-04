#!/usr/bin/env python3
"""Read-only M0 Stage 2 security and data-integrity comparisons."""
import argparse
import hashlib
import json
import os
from pathlib import Path
import re
import subprocess
import sys

def sql(url, statement):
    p = subprocess.run(["psql", "-X", "-q", "-A", "-t", "-v", "ON_ERROR_STOP=1",
                        "-d", url, "-c", statement], capture_output=True, text=True)
    if p.returncode:
        raise RuntimeError("PostgreSQL query failed: " + p.stderr[-350:])
    return p.stdout.strip()

def query(url, statement):
    result = sql(url, "SELECT coalesce(json_agg(x)::text,'[]') FROM (" + statement + ") x")
    return json.loads(result)

def main():
    p = argparse.ArgumentParser()
    p.add_argument("--source-url", default=os.environ.get("M0_SOURCE_URL"))
    p.add_argument("--target-url", default=os.environ.get("M0_TARGET_URL"))
    p.add_argument("--output", default="reports")
    args = p.parse_args()
    if not args.source_url or not args.target_url:
        p.error("Both M0 database URLs are required")
    report = {"stage": 2, "status": "FAIL", "checks": []}
    def check(name, func):
        try:
            result = func()
            report["checks"].append({"name": name, "status": "PASS", "detail": result})
        except Exception as exc:
            report["checks"].append({"name": name, "status": "FAIL", "detail": str(exc)[:400]})
    def same(name, statement):
        def verify():
            a = query(args.source_url, statement)
            b = query(args.target_url, statement)
            if a != b:
                raise AssertionError("Source and recovery results differ")
            return {"matching_records": len(a)}
        check(name, verify)
    same("schema and table ACLs", """
        SELECT 'schema' AS kind, n.nspname AS object,
          coalesce(n.nspacl::text,'<default>') AS acl
        FROM pg_namespace n WHERE n.nspname='m0_fixture'
        UNION ALL
        SELECT 'relation', c.relname, coalesce(c.relacl::text,'<default>')
        FROM pg_class c JOIN pg_namespace n ON n.oid=c.relnamespace
        WHERE n.nspname='m0_fixture' AND c.relkind IN ('r','p','S','v')
        ORDER BY 1,2
    """)
    same("role privilege matrix", """
        SELECT r.rolname AS role, 'schema' AS object,
          has_schema_privilege(r.oid,n.oid,'USAGE') AS can_use,
          has_schema_privilege(r.oid,n.oid,'CREATE') AS can_create,
          false AS can_select
        FROM pg_roles r CROSS JOIN pg_namespace n
        WHERE r.rolname IN ('anon','authenticated') AND n.nspname='m0_fixture'
        UNION ALL
        SELECT r.rolname, c.relname,
          false, false, has_table_privilege(r.oid,c.oid,'SELECT')
        FROM pg_roles r CROSS JOIN pg_class c
        JOIN pg_namespace n ON n.oid=c.relnamespace
        WHERE r.rolname IN ('anon','authenticated')
          AND n.nspname='m0_fixture' AND c.relkind IN ('r','p')
        ORDER BY 1,2
    """)
    def data_integrity():
        tables = query(args.source_url, """
            SELECT tablename FROM pg_tables WHERE schemaname='m0_fixture' ORDER BY tablename
        """)
        if not tables:
            raise AssertionError("No fixture tables found")
        target_tables = query(args.target_url, """
            SELECT tablename FROM pg_tables WHERE schemaname='m0_fixture' ORDER BY tablename
        """)
        if tables != target_tables:
            raise AssertionError("Fixture table lists differ")
        for item in tables:
            name = item["tablename"]
            if not re.fullmatch(r"[A-Za-z_][A-Za-z_0-9]*", name):
                raise AssertionError("Unexpected fixture table name")
            statement = ('COPY (SELECT row_to_json(t)::text AS value FROM '
                         '"m0_fixture"."' + name + '" t ORDER BY value) TO STDOUT')
            hashes = []
            for url in (args.source_url, args.target_url):
                proc = subprocess.run(["psql","-X","-q","-v","ON_ERROR_STOP=1",
                                       "-d",url,"-c",statement],
                                      stdout=subprocess.PIPE, stderr=subprocess.PIPE)
                if proc.returncode:
                    raise RuntimeError("Data read failed: " + proc.stderr.decode(errors="replace")[-300:])
                hashes.append(hashlib.sha256(proc.stdout).hexdigest())
            if hashes[0] != hashes[1]:
                raise AssertionError("Data differs in table " + name)
        return {"matching_tables": len(tables)}
    check("full fixture row contents", data_integrity)
    same("sequence states", """
        SELECT schemaname, sequencename, last_value
        FROM pg_sequences WHERE schemaname='m0_fixture' ORDER BY sequencename
    """)
    def rls_behaviour():
        statement = """BEGIN;
            SET LOCAL ROLE authenticated;
            SELECT count(*) FROM m0_fixture.parent;
            ROLLBACK;"""
        values = []
        for url in (args.source_url, args.target_url):
            values.append(sql(url, statement).splitlines())
        if values[0] != values[1]:
            raise AssertionError("Authenticated row visibility differs")
        if not any(line.strip() == '0' for line in values[0]):
            raise AssertionError("Expected fixture denied-row result was not observed")
        return {"authenticated_visible_rows": 0}
    check("authenticated RLS denied-row behaviour", rls_behaviour)
    def role_visibility():
        # Read-only impersonation: verify both restricted roles and the
        # privileged fixture baseline, on source and recovery.
        privileged = []
        for url in (args.source_url, args.target_url):
            privileged.append(int(sql(url, "SELECT count(*) FROM m0_fixture.parent")))
        if privileged[0] != privileged[1] or privileged[0] < 1:
            raise AssertionError("Fixture must contain matching non-empty privileged baseline")
        results = {}
        for role in ("anon", "authenticated"):
            counts = []
            for url in (args.source_url, args.target_url):
                statement = ("BEGIN; SET LOCAL ROLE " + role +
                             "; SELECT count(*) FROM m0_fixture.parent; ROLLBACK;")
                lines = sql(url, statement).splitlines()
                numbers = [int(line.strip()) for line in lines if line.strip().isdigit()]
                if len(numbers) != 1:
                    raise AssertionError("Unexpected impersonated-role query result for " + role)
                counts.append(numbers[0])
            if counts[0] != counts[1] or counts[0] != 0:
                raise AssertionError(role + " must see zero fixture rows in both databases")
            results[role] = counts[0]
        return {"privileged_rows": privileged[0], "restricted_role_rows": results}
    check("restricted and privileged RLS behaviour", role_visibility)
    report["status"] = "PASS" if all(x["status"] == "PASS" for x in report["checks"]) else "FAIL"
    output = Path(args.output)
    output.mkdir(parents=True, exist_ok=True)
    path = output / "m0-stage2.json"
    path.write_text(json.dumps(report, indent=2) + "\n")
    print(report["status"] + ": " + str(path))
    return 0 if report["status"] == "PASS" else 1

if __name__ == "__main__":
    sys.exit(main())

#!/usr/bin/env python3
"""Specification quality gates. Standard library except PyYAML and jsonschema."""
import csv
import json
import re
import sqlite3
import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
errors = []

def check(condition, message):
    if not condition:
        errors.append(message)

def load_yaml(path):
    import yaml
    with path.open(encoding="utf-8") as f:
        return yaml.safe_load(f)

def check_ids(path, pattern):
    text = path.read_text(encoding="utf-8")
    ids = re.findall(pattern, text)
    check(bool(ids), f"{path}: no requirement IDs found")
    return set(ids)

for p in ["README.md", "SECURITY.md", "CONTRIBUTING.md",
          "docs/requirements/BASELINE.md", "docs/architecture/DECISIONS.md",
          "docs/testing/RELEASE_GATES.md"]:
    check((ROOT / p).is_file(), f"Missing {p}")

baseline = ROOT / "docs/requirements/BASELINE.md"
if baseline.is_file():
    check_ids(baseline, r"REQ-[A-Z]+-\d{3}")

spec = ROOT / "docs/spec/v1.0.0"
if spec.exists():
    required = ["requirements/SRS.md", "requirements/traceability.csv",
                "architecture/ARCHITECTURE.md", "api/openapi.yaml",
                "data/MODEL.md", "data/schema.sql",
                "schemas/recovery-set.schema.json",
                "testing/TEST_PLAN.md", "testing/test-cases.csv",
                "releases/RELEASE_PLAN.md", "decisions/ADR-0001.md"]
    for name in required:
        check((spec / name).is_file(), f"Missing complete specification file {name}")

    trace = spec / "requirements/traceability.csv"
    tests = spec / "testing/test-cases.csv"
    if trace.is_file() and tests.is_file():
        with trace.open(newline="", encoding="utf-8-sig") as f:
            rows = list(csv.DictReader(f))
        with tests.open(newline="", encoding="utf-8-sig") as f:
            test_rows = list(csv.DictReader(f))
        req_ids = [r.get("requirement_id", "") for r in rows]
        test_ids = [r.get("test_id", "") for r in test_rows]
        check(len(req_ids) == len(set(req_ids)), "Duplicate requirement IDs")
        check(len(test_ids) == len(set(test_ids)), "Duplicate test IDs")
        check(len(req_ids) >= 73, "Expected at least 73 traced requirements")
        check(len(test_ids) >= 16, "Expected at least 16 test cases")
        known_tests = set(test_ids)
        for row in rows:
            rid = row.get("requirement_id", "")
            check(bool(row.get("acceptance_criterion")), f"{rid}: missing acceptance criterion")
            check(row.get("priority") in {"P0", "P1", "P2"}, f"{rid}: invalid priority")
            linked = [x.strip() for x in row.get("verification", "").split(",") if x.strip()]
            check(bool(linked), f"{rid}: missing verification tests")
            for tid in linked:
                check(tid in known_tests, f"{rid}: unknown test {tid}")
    api = spec / "api/openapi.yaml"
    if api.is_file():
        try:
            contract = load_yaml(api)
            check(str(contract.get("openapi", "")).startswith("3.1"), "OpenAPI must be 3.1")
            check(bool(contract.get("paths")), "OpenAPI paths missing")
        except Exception as exc:
            errors.append(f"OpenAPI invalid: {exc}")
    schema = spec / "schemas/recovery-set.schema.json"
    if schema.is_file():
        try:
            import jsonschema
            jsonschema.Draft202012Validator.check_schema(json.loads(schema.read_text()))
        except Exception as exc:
            errors.append(f"Recovery manifest JSON Schema invalid: {exc}")
    sql = spec / "data/schema.sql"
    if sql.is_file():
        try:
            con = sqlite3.connect(":memory:")
            con.executescript(sql.read_text())
            check(con.execute("PRAGMA integrity_check").fetchone()[0] == "ok",
                  "SQLite integrity check failed")
            con.close()
        except Exception as exc:
            errors.append(f"SQLite schema invalid: {exc}")
else:
    print("NOTICE: complete specification has not yet been imported. Baseline-only checks run.")

if errors:
    print("Specification validation failed:")
    for e in errors:
        print(" -", e)
    sys.exit(1)
print("Specification validation passed.")

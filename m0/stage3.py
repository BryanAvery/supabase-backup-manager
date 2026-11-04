#!/usr/bin/env python3
"""Offline M0 acceptance gate: requires two complete, successful validation cycles."""
import argparse
import datetime as dt
import json
from pathlib import Path
import sys

REQUIRED_STAGE1 = {
    "distinct database identities", "source schema exists", "reset managed target fixture", "target schema absent",
    "backup", "archive readable", "restore", "mark managed fixture",
    "schema fingerprint", "table row counts", "RLS flags", "RLS policies"
}
REQUIRED_STAGE2 = {
    "schema and table ACLs", "role privilege matrix", "full fixture row contents",
    "sequence states", "authenticated RLS denied-row behaviour",
    "restricted and privileged RLS behaviour"
}

def load(path):
    data = json.loads(path.read_text())
    if not isinstance(data, dict):
        raise ValueError("Invalid report object")
    return data

def validate(report, required, label):
    checks = report.get("checks", [])
    if report.get("status") != "PASS":
        raise ValueError(label + " status is not PASS")
    names = {c.get("name") for c in checks if c.get("status") == "PASS"}
    missing = required - names
    if missing:
        raise ValueError(label + " missing passing checks: " + ", ".join(sorted(missing)))

def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--reports", default="reports")
    args = ap.parse_args()
    directory = Path(args.reports)
    output = directory / "m0-stage3-acceptance.json"
    result = {
        "stage": 3, "status": "NOT_ACCEPTED",
        "generated_at": dt.datetime.now(dt.timezone.utc).isoformat(),
        "criteria": "Two consecutive successful Stage 1 and Stage 2 cycles",
        "cycles": []
    }
    try:
        for cycle in (1, 2):
            stage1_path = directory / f"cycle-{cycle}-stage1.json"
            stage2_path = directory / f"cycle-{cycle}-stage2.json"
            stage1 = load(stage1_path)
            stage2 = load(stage2_path)
            validate(stage1, REQUIRED_STAGE1, f"Cycle {cycle} Stage 1")
            validate(stage2, REQUIRED_STAGE2, f"Cycle {cycle} Stage 2")
            result["cycles"].append({"cycle": cycle, "stage1": "PASS", "stage2": "PASS"})
        result["status"] = "PASS"
        result["note"] = "Automated fixture acceptance only; full Supabase recovery and JWT/API access tests remain outside scope."
    except Exception as exc:
        result["reason"] = str(exc)[:500]
    directory.mkdir(parents=True, exist_ok=True)
    output.write_text(json.dumps(result, indent=2) + "\n")
    print(result["status"] + ": " + str(output))
    return 0 if result["status"] == "PASS" else 1

if __name__ == "__main__":
    sys.exit(main())

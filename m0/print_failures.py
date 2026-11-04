#!/usr/bin/env python3
"""Emit only failed check names from an M0 JSON report in GitHub Actions."""
import json
import sys
from pathlib import Path

def main():
    report = json.loads(Path(sys.argv[1]).read_text())
    failures = [item.get("name", "unnamed check")
                for item in report.get("checks", [])
                if item.get("status") != "PASS"]
    if failures:
        for name in failures:
            # Do not print database errors or URLs, which may contain credentials.
            print("::error::M0 check failed: " + str(name).replace("\n", " "))
    else:
        print("::error::M0 validation failed before a named check completed")
    print("See the uploaded JSON report for further details.")

if __name__ == "__main__":
    main()

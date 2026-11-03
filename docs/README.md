# Engineering baseline

**Baseline version:** 1.0.0 (initial governance draft). The detailed engineering specification previously prepared offline must be imported and reviewed before this baseline can be declared complete.

The documentation hierarchy is:
1. `requirements/BASELINE.md` - mandatory product requirements and traceability conventions.
2. `architecture/DECISIONS.md` - architectural constraints and decisions.
3. `testing/RELEASE_GATES.md` - evidence required to release.
4. `../SECURITY.md` - vulnerability handling and secrets policy.

## Change control
Each requirement has a stable identifier, priority, rationale, acceptance test and release milestone. Never reuse deleted identifiers. A PR changing behaviour must identify affected requirements, update tests and amend the changelog. Architecture deviations require a new ADR. A passing CI workflow is necessary but not sufficient for release approval.

## Status
This directory establishes governance controls. The complete 73-requirement SRS, OpenAPI contract, SQL schema, recovery manifest, traceability matrix and test-case catalogue from the engineering specification package are **pending import** and must not be described as already committed.

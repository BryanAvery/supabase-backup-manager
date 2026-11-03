# Complete engineering specification import checklist

The full engineering package was created separately and must be imported **verbatim**, not reconstructed from summaries.

Target location: `docs/spec/v1.0.0/`.

Expected source files:
- `requirements/SRS.md`
- `requirements/traceability.csv`
- `architecture/ARCHITECTURE.md`
- `api/openapi.yaml`
- `data/MODEL.md`
- `data/schema.sql`
- `schemas/recovery-set.schema.json`
- `testing/TEST_PLAN.md`
- `testing/test-cases.csv`
- `releases/RELEASE_PLAN.md`
- `decisions/ADR-0001.md`
- `README.md`, `VERSIONING.md`, `CHANGELOG.md`

After import, run `python -m pip install PyYAML jsonschema` and `python scripts/validate_spec.py`. Verify every requirement has a unique ID, a mapped test and an acceptance criterion. Review naming differences between the initial abbreviated governance baseline and the original SRS before claiming a single authoritative requirements catalogue.

Do not mark this task complete until all expected source files are committed and CI passes.

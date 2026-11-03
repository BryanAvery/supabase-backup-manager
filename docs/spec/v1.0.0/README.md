# Supabase Backup Manager - engineering specification pack

**Baseline:** 1.0.0 | **Date:** 2026-11-03 | **Status:** proposed, not implemented | **Owner:** product owner (to appoint)

This repository-ready pack is a **specification**, not working application code. It uses the previously agreed SRS as the authoritative requirement baseline. All other files are subordinate and should be reviewed together.

## Documents
- `requirements/SRS.md`: normative requirements and acceptance criteria.
- `requirements/traceability.csv`: machine-readable requirement-to-test map.
- `architecture/ARCHITECTURE.md`: C4-style views, security boundaries, lifecycle and recovery workflows.
- `data/MODEL.md` and `data/schema.sql`: logical and executable SQLite catalogue baseline.
- `api/openapi.yaml`: versioned HTTP API contract and examples.
- `schemas/recovery-set.schema.json`: portable manifest validation contract.
- `testing/TEST_PLAN.md` and `testing/test-cases.csv`: evidence-driven validation.
- `releases/RELEASE_PLAN.md`: gated engineering increments and production criteria.
- `decisions/ADR-0001.md`: architectural decisions and open risks.
- `CHANGELOG.md`, `VERSIONING.md`: change control.

## Governance
Use Git tags `spec/v1.0.0`, PR review and CODEOWNERS in a future Git repository. Do not claim a Git tag or branch exists merely because this package includes these instructions.

## Critical constraint
Supabase managed settings, Auth secret material and deployed Edge Function source may not be exportable. The system must record actual capability and require an externally maintained recovery source or tested manual procedure. No automatic recovery guarantee is implied.

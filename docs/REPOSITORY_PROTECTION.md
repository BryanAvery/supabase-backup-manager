# GitHub repository protection

**Status:** Required administrator action. The connected GitHub integration cannot modify branch protection or repository rulesets.

## Required settings
In **Settings → Rules → Rulesets**, create an active branch ruleset targeting `main`:
- Require pull requests before merging.
- Require status checks to pass: `validate` from **Specification validation**.
- Require branches to be up to date before merging.
- Block force pushes and deletions.
- Require conversation resolution.
- Prevent bypass except emergency administrators with an audited procedure.

For a single-maintainer repository, start with **zero required external approvals** to avoid locking out the only maintainer. Once another maintainer joins, require one independent approval and code-owner review.

Enable **Settings → Security → Code security**: secret scanning, push protection and Dependabot alerts where available. Enable **Settings → Security → Private vulnerability reporting** if available.

## Import the full specification
Download the complete engineering specification archive and use the import script from the handover kit to commit all 14 original files to `docs/spec/v1.0.0` on `docs/engineering-baseline-v1`. Check the workflow and review before merging PR #1.

## Verification
- Confirm the branch ruleset is active for `main`.
- Confirm the required `validate` check appears on PR #1.
- Confirm direct pushes and force pushes to `main` are rejected.
- Confirm the complete SRS, OpenAPI, SQLite schema, recovery manifest and test traceability are present.
- Confirm the specification validation workflow passes.

# M0 automated backup and restore validation

M0 is a **fixture-level proof of recovery**, not a complete Supabase project backup. The runner uses PostgreSQL tools to back up the `m0_fixture` schema from a source database, restore it to a separate recovery database and compare selected results.

## GitHub Actions

Workflow: `.github/workflows/m0-validation.yml` (manual `workflow_dispatch` only).

1. Merge the workflow pull request into `main`.
2. In repository **Settings > Environments**, create `m0-recovery`.
3. Add encrypted environment secrets `M0_SOURCE_URL` and `M0_TARGET_URL` containing the PostgreSQL connection strings. Never commit credentials or paste them into issues.
4. Confirm the target is a **distinct, disposable recovery database**. The script compares URL strings only; this does **not** prove the URLs resolve to different databases. Check project/database identities and credentials out of band before running.
5. Go to **Actions > M0 backup and restore validation > Run workflow** on the branch containing the workflow.
6. Review the job outcome and download the `m0-validation-report` JSON artefact (14-day retention).

GitHub-hosted runners must be able to reach both PostgreSQL endpoints, and the database users need sufficient privileges. The workflow serialises executions and has a 20-minute timeout.

## Cleanup policy: important

**Never wipe both databases.** The source database is read-only for this workflow and must retain its `m0_fixture` fixture. The recovery database is the only cleanup target.

With `--reset-managed-fixture`, the runner checks the recovery database for `m0_fixture`:
- If absent, it proceeds.
- If the schema comment is exactly `m0-runner-managed-v1`, it executes `DROP SCHEMA m0_fixture CASCADE` and then restores the fixture.
- If the schema exists but has no matching comment, it **fails closed** without deleting it.

After a successful restore, the runner sets the marker comment. The marker is not cryptographic proof of ownership, so **only use this on an explicitly dedicated recovery database**. `CASCADE` may remove objects depending on the fixture schema, including dependent objects outside it; isolate the target and review dependencies. The runner does not clean unrelated schemas such as `public`, `auth` or `storage` intentionally. It does not delete the source fixture.

A failed run may leave a partial, unmarked schema. Do not automatically delete it: inspect the recovery database and resolve the state deliberately before retrying.

## Local execution

Requires Python 3.10+ and compatible PostgreSQL client binaries (`psql`, `pg_dump`, `pg_restore`).

```bash
export M0_SOURCE_URL='postgresql://...'
export M0_TARGET_URL='postgresql://...'
python3 m0/run_m0.py --confirm-isolated-target --reset-managed-fixture --output reports
```

Omit `--reset-managed-fixture` to refuse any existing target fixture. The `--confirm-isolated-target` flag is an operator assertion, not an automated identity check. Connection URLs passed to PostgreSQL utilities can be visible in process arguments; use a secure runner and restrict access.

## Checks and evidence

The runner verifies source schema existence, target absence after optional cleanup, `pg_dump` success, archive readability, `pg_restore --no-owner --no-acl` success, schema-column fingerprint, table row counts, RLS enable/force flags and RLS policy definitions. Reports are written to `reports/m0-<timestamp>.json`. A failing check exits non-zero.

**Limitations:** ACLs and role grants are not restored by this runner; authorised/unauthorised user behaviour and JWT-based RLS are not tested. It does not verify complete row contents, sequences, extensions, auth/storage objects or full-project recovery. The database tests have not yet been run and verified. A successful workflow does not mean M0 is fully complete.

## Recovery and troubleshooting

- Missing secrets: configure both secrets in the `m0-recovery` environment.
- Connection failure: check database networking, credentials, TLS and GitHub runner reachability.
- Existing unmarked fixture: stop and inspect; do not blindly drop it.
- Schema mismatch: inspect the JSON report and PostgreSQL restore errors.
- Before any destructive operation, independently confirm the recovery project identity and that no valuable data depends on the fixture schema.

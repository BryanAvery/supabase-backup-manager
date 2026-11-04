# M0 automated validation (initial runner)

This is an **initial, non-destructive** PostgreSQL fixture backup/restore check. It does not yet replace the manual ACL and impersonated-role tests.

## Requirements
- Python 3.10+
- PostgreSQL client binaries: `psql`, `pg_dump`, `pg_restore`
- Two separate database connections: a source containing `m0_fixture`, and an **isolated disposable target database** where `m0_fixture` does not exist
- Appropriate PostgreSQL privileges. Do not use a production database as the target.

## Run
```bash
export M0_SOURCE_URL='postgresql://...'
export M0_TARGET_URL='postgresql://...'
python3 m0/run_m0.py --confirm-isolated-target
```

Avoid committing credentials. The script never intentionally prints connection URLs. The target must be checked by the operator; different URL strings alone do not prove different databases. Prefer credential handling that avoids exposing passwords in process arguments.

## Current checks
1. Source fixture exists and target fixture does not
2. `pg_dump` creates a custom-format archive
3. `pg_restore --list` reads the archive
4. Restore succeeds with `--no-owner --no-acl`
5. Schema column fingerprint matches
6. Table row counts match
7. RLS enable/force flags match
8. RLS policy definitions match

A JSON report is written to `reports/`. The runner exits non-zero on failure. It does not drop or overwrite schemas.

## Not yet covered
- ACL and role grants (deliberately excluded from restore in this first pass)
- Actual authorised/unauthorised role and JWT/RLS behaviour
- Row-level data checksums and sequence values
- Extensions, storage objects, auth configuration and full-project recovery
- CI execution against an ephemeral PostgreSQL target

Do not mark M0 complete until these tests are implemented and run successfully.

## Stage 2: security and integrity (experimental)

After a successful Stage 1 restore, GitHub Actions runs `python3 m0/stage2.py --output reports`. The script is read-only and produces `reports/m0-stage2.json`. A failing check makes the job fail.

Checks:
- Compare raw schema/table ACL definitions and effective `anon`/`authenticated` privilege matrices.
- Compare SHA-256 hashes of all fixture table rows in deterministic JSON-text order (the row content, not only row counts).
- Compare sequence `last_value` visible through `pg_sequences`.
- Run `SET LOCAL ROLE authenticated` and check the expected zero visible rows in `m0_fixture.parent` on both databases.

**Known gaps and expected failures:** Stage 1 restores with `--no-acl`, so ACL checks may fail until permissions are restored explicitly. The RLS test currently covers only the fixture's denied-row case for `authenticated`; it does not prove allowed-row access, `anon` restrictions, JWT claims or API-level authentication. The sequence check does not cover `is_called` or inaccessible sequence values. Raw ACL strings can differ because owners differ. This is a first automated Stage 2 implementation, **not** final security acceptance. Review failing reports and extend tests before signing off M0.

No databases have been modified or tests executed by adding this code.


## Stage 3: repeatability and automated acceptance gate

The manual GitHub Actions workflow now runs **two consecutive Stage 1 and Stage 2 cycles** against the same dedicated recovery target. Stage 1's marker-controlled reset prepares the target for the second restore. Each successful cycle produces separate `cycle-1-stage1.json`, `cycle-1-stage2.json`, `cycle-2-stage1.json` and `cycle-2-stage2.json` evidence files. The `m0/stage3.py` gate checks both reports for each cycle and writes `m0-stage3-acceptance.json`.

**Acceptance rule:** all required named checks in both cycles must pass. Missing reports, failed checks, or an incomplete second cycle result in `NOT_ACCEPTED` and a failed workflow. Evidence is uploaded even when the workflow fails. GitHub Actions is still manually triggered, not scheduled.

**Scope:** a `PASS` means the automated M0 *fixture-level* acceptance criteria passed twice. It does **not** certify complete Supabase disaster recovery, JWT/API-level access, extension restoration or storage/auth backup. Stage 2 currently has known gaps and Stage 1 excludes ACL restoration, so failures are expected until those are resolved. Do not treat this gate as sign-off for those untested areas.

**Safety:** only use a separately verified disposable recovery database. The marker-controlled reset drops `m0_fixture` with `CASCADE` when previously marked by the runner; review dependencies. The source fixture is not cleaned. Do not trigger this workflow until the target identity and secrets are confirmed.

Run the offline acceptance check against existing reports with `python3 m0/stage3.py --reports reports`. It does not connect to or modify databases.


## M0 hardening: identity, ACL restore and role checks

- Before **any target cleanup**, Stage 1 queries `pg_control_system()` for the PostgreSQL cluster system identifier and combines it with the current database OID. If the identities match, or if the query is unavailable, the run stops without cleanup. Connection URL inequality is not treated as proof of isolation.
- Stage 1 now restores ACLs (it no longer uses `--no-acl`), while still avoiding owner restoration. The target must already have referenced roles, and ACL restore may fail if grants require unavailable roles or privileges. Such a failure is intentional, not silently ignored.
- Stage 2 checks `anon` and `authenticated` see zero fixture rows, and verifies that the privileged fixture baseline is non-empty and matches between source and recovery.
- Stage 3 requires both the identity and expanded role checks in each cycle.

**Remaining acceptance limitations:** These role tests do not simulate a genuine signed-in Supabase user with JWT claims or prove that a permitted authenticated user can access their own row. They also do not verify application/API authentication, or complete Supabase disaster recovery. A full positive JWT/RLS test requires a fixture with a documented permitted user and claims. The database-identity query requires permission to call `pg_control_system()`; if denied, request an approved safe identity mechanism rather than bypassing the check. Confirm the recovery target is disposable and has no external dependencies before running the marker-controlled `DROP SCHEMA ... CASCADE`.

## Failure diagnostics

The workflow now saves the Stage 1 or Stage 2 JSON report for a failed cycle before exiting. It emits a GitHub Actions error annotation naming the failed check without echoing database credentials or connection strings. The Stage 3 acceptance gate still fails closed if a cycle is incomplete, and the artifact upload runs even on failure. For a failed run, download `m0-validation-report` and inspect `cycle-1-stage1.json` or `cycle-1-stage2.json` to identify the underlying error. Do not share unredacted reports publicly because database error messages may contain connection details.

## Supabase-hosted identity verification

If the provider blocks `pg_control_system()`, M0 now accepts **only** two Supabase session-pooler connection URLs with different `postgres.<20-character-project-ref>` usernames and `*.pooler.supabase.com` hosts. The pooler project reference determines routing to a Supabase project. A matching project reference, unsupported connection form or ambiguous URL fails before target cleanup. This fallback checks project routing information in the connection URLs, not an independently retrieved database identifier. Operators must still verify that the target project is disposable and separate from the source. For direct database connections, use the PostgreSQL system identity check or obtain an approved independent identity mechanism; do not bypass the guard.

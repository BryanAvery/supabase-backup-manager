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

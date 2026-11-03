# Verification and validation plan - 1.0.0

## Test philosophy
Test actual recovery, not only backup command success. Every P0 requirement must map to a test case and stored evidence. Use disposable, synthetic projects only; never run destructive tests on production. Include both Supabase Cloud and a pinned representative self-hosted topology.

## Test levels
1. Static: SRS/traceability lint, OpenAPI 3.1 validation, JSON Schema validation, SQLite migration and FK integrity.
2. Unit: state machines, scheduler DST, RBAC policy, canonical manifest, crypto envelope, retention decisions.
3. Contract: provider adapter mocks, pagination, API error envelopes, idempotency, approval digests.
4. Integration: real PostgreSQL and Storage; Cloud API and self-hosted API with sandbox credentials.
5. End-to-end: full project rebuild, Auth login, function invocation, Storage object hashes, app smoke tests.
6. Security: OWASP ASVS-inspired auth tests, secrets scan, supply-chain scan, API fuzz, tamper injection.
7. Resilience: kill daemon, disconnect network, fill disk, expire credentials, remove Mac host, corrupt catalogue.
8. Release: upgrade N-1, restore old archives, code signing/notarisation, macOS accessibility and web WCAG 2.2 AA.

## Environments and fixtures
- CI: SQLite temporary DB, Go unit tests, React/Swift client generation, schema/OpenAPI linters.
- Integration: PostgreSQL versions per supported matrix, synthetic schemas (FKs, RLS, views, extensions, sequences, large objects), 100k-row test table, paginated Storage objects, fake OAuth secrets.
- DR lab: clean Supabase Cloud test project and disposable self-hosted deployment, separate restore host, fresh credentials, escrowed keys.
- Scale: >=20 registered projects with bounded concurrency; datasets and expected timings recorded in benchmark baseline.

## Evidence requirements
For each run store: test ID, build SHA, environment versions, fixture hash, time, operator, outcome, logs with redaction, manifest IDs, archive hashes, restore checks and measured RPO/RTO. Store evidence in restricted immutable artefact repository; redact personal data and secrets.

## Explicit pass/fail
- Database: row/object/permission inventories match defined supported subset; incompatible objects are reported, not silently skipped.
- Storage: every expected object matches checksum and bucket configuration; missing objects block pass.
- Auth: test users can sign in through configured supported flows; secret-dependent manual steps explicitly complete.
- Functions: deployment source revision verified; smoke invocation succeeds.
- Settings: all automatable settings match expected; manual configuration checklist signed off.
- Host loss: portable CLI restores without original Mac, Keychain or catalogue.
- Tampering: corrupted archive or manifest cannot be decrypted/published as valid.
- Permissions: cross-project requests always rejected, including ID enumeration and SSE.
- Restore safety: destructive production request without plan digest, safety backup or required approval cannot run.

## Release policy
Engineering milestones M0-M6 may be delivered as preview builds; **no preview build is described as production-ready**. Production R1 requires all P0 cases passed for supported environments, all mandatory six component categories assessed, documented manual gaps accepted explicitly, full Cloud and self-hosted drills, fresh-host recovery, signed binaries, no unresolved critical/high exploitable security issues, and operations sign-off. A failed mandatory test blocks release; waiver requires formal product/security approval and cannot create a false claim of complete recovery.

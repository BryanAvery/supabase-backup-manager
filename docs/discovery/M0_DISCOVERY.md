# M0 technical discovery - evidence-led assessment
Date: 3 November 2026. **Status: NOT COMPLETE / NO-GO.** Isolated Cloud PostgreSQL fixture, manual single-object Storage recovery, and synthetic email/password Auth recovery drills have passed. Full Cloud and self-hosted recovery are not yet demonstrated; no production project was modified.

## Executed Cloud PostgreSQL fixture recovery - 3 November 2026

**Result: PASS for isolated `m0_fixture` logical backup/restore only; overall M0 remains NO-GO.**

- Source: disposable Supabase Cloud project `sbm-m0-cloud-source` (`qfcowkrbothbseswosjg`).
- Destination: independent disposable Supabase Cloud project `sbm-m0-cloud-restore` (`fdlozdsycfeaxglwstjx`).
- Region: London (`eu-west-2`); source PostgreSQL 17.11; local `pg_dump`/`pg_restore` 18.6.
- Source backup: `pg_dump -W --format=custom --no-owner --no-acl --schema=m0_fixture --file=$HOME/supabase-m0/backups/m0_fixture_new.dump`. File 6.9 KB, custom archive dump version 1.16-0, 25 TOC entries, gzip compressed.
- Archive inspection: `pg_restore --list` showed schema, two tables, two sequences, a view, table data, sequence values, primary/unique constraints, an index, a foreign key, RLS enablement and two policies.
- Destination connection authenticated using the project-specific Session pooler user `postgres.fdlozdsycfeaxglwstjx`. Only the pre-existing synthetic destination `m0_fixture` schema was dropped with `DROP SCHEMA m0_fixture CASCADE`.
- Restore: `pg_restore -W --verbose --exit-on-error --no-owner --no-acl --dbname=postgres .../m0_fixture_new.dump`; all reported object creation/data load steps completed, with no errors reported.
- Destination checks: 3 parent rows; 4 child rows; 2 RLS policies; 1 foreign key.
- Source/destination equality, using identical ordered MD5 expressions over IDs, labels, parent IDs and payloads:
  - parent: `0a7083c301db6a7a2125751664873464` on both;
  - child: `5955fa9eb3c8a55d5937dab96399f744` on both.
- Evidence: operator-provided Terminal outputs in the M0 discovery session. The dump is on the operator's Mac, **not committed to Git**.
- Limits: this is a schema-scoped fixture restore, not full project or database recovery; database roles/grants, full Auth configuration, bulk Storage object bytes, Edge Function source, project settings, secrets, fresh-host recovery, sequence increment behaviour, negative tests, RPO/RTO and consistency remain unverified. MD5 comparisons cover selected fixture fields, not all metadata or bytes.

**Next drill:** prove clean-target Edge Function recovery using an independently retained source artifact and a new synthetic function name; then expand Storage recovery to paginated bulk objects and denied-access tests. Keep real user data and credentials out of evidence and Git.

## Executed Cloud Storage single-object recovery - 3 November 2026

**Result: PASS for manual recovery of one private Storage object; full bucket backup/recovery remains unverified.**

- Source: disposable Cloud project `qfcowkrbothbseswosjg`; destination: independent disposable Cloud project `fdlozdsycfeaxglwstjx`.
- Source bucket `m0-backup-test` and destination bucket `m0-backup-test` were both verified in `storage.buckets` with `public = false`.
- Object `recovery-test.txt` was verified in both projects' `storage.objects` metadata: 453 bytes, `text/plain`.
- Operator downloaded the source object through the Supabase dashboard, retained a local copy, manually created the private destination bucket and uploaded that copy.
- Source SHA-256: `01813332407a69e1a5683271b60ac0adce259932ff516632036d60efa3522842`.
- Operator downloaded the destination object independently and calculated SHA-256: `01813332407a69e1a5683271b60ac0adce259932ff516632036d60efa3522842`.
- **Byte-for-byte equality confirmed** by matching SHA-256 digests, with matching object size and bucket privacy.
- Scope limitations: single-object manual operation only; no paginated enumeration, bulk export/restore, access-policy equivalence, denied anonymous access test, version handling, retention, automation, retry/resume, or RPO/RTO measurement. Private bucket metadata alone does not prove every access-control policy. No production data used.
- Synthetic Auth user `m0-test@example.com` was separately confirmed in source `auth.users` with confirmed email. The subsequent controlled Auth restore and sign-in are recorded below.

## Executed Cloud Auth email/password recovery - 3 November 2026

**Result: PASS for synthetic email/password user and identity data restore with successful password sign-in; full Auth configuration recovery remains unverified.**

- Source disposable project `qfcowkrbothbseswosjg`; independent disposable destination `fdlozdsycfeaxglwstjx`.
- Source synthetic user: `m0-test@example.com`; original UUID `a53144f6-ee4a-41ea-9aa4-259d7ef49a18`, confirmed email and one email identity.
- Source backup: `pg_dump -W --format=custom --data-only --no-owner --no-acl --table=auth.users --table=auth.identities --file=$HOME/supabase-m0/backups/auth/m0_auth.dump`. Archive 2.5 KB, gzip, dump version 1.16-0, PostgreSQL source 17.11, pg_dump 18.6.
- Archive TOC contained only two TABLE DATA entries: `auth.users` and `auth.identities`. Local archive permissions restricted with `chmod 600`; archive not committed to Git.
- Before restore, independent destination had zero Auth users and zero Auth identities.
- Destination restore used `pg_restore -W --verbose --single-transaction --exit-on-error --data-only --no-owner --no-acl --dbname=postgres .../m0_auth.dump` with destination-specific Session pooler credentials. Terminal reported both table-data operations with no errors.
- Post-restore destination SQL confirmed original UUID, confirmed email, nonempty password hash, and one email identity. No password hashes or secrets included in Git evidence.
- Operator submitted original test email/password to the **destination** Auth password-grant endpoint (`/auth/v1/token?grant_type=password`) using the destination publishable/anon key. Response contained `user_id = a53144f6-ee4a-41ea-9aa4-259d7ef49a18` and `error = null`. Access tokens and passwords were not recorded.
- **Proven:** identity and password-based login survive this narrow cross-project data restore.
- **Not proven:** full Auth schema/config restore, OAuth/SAML/MFA, refresh-token/session continuity, email templates/SMTP, redirect/provider configuration, external secrets, triggers and cross-service dependencies, multi-user recovery, fresh-host execution, RPO/RTO, or repeatability under automation. This manual data-only method must not be presented as a general-purpose production Auth recovery procedure.

## Executed Cloud Edge Function invocation - 3 November 2026

**Result: PASS for authenticated invocation of an existing destination deployment; clean-target Edge Function recovery NOT YET PROVEN.**

- Source disposable project `qfcowkrbothbseswosjg` and destination disposable project `fdlozdsycfeaxglwstjx` both already contained active Edge Function `m0-fixture` (version 2, `verify_jwt = true`) when inspected.
- The source and destination function files `index.ts` had identical code returning JSON `{"fixture":"m0","version":1,"ok":true}`. Deployment bundle SHA-256 metadata differed; no assertion of identical deployed bundles.
- Operator obtained a destination Auth access token using the restored synthetic email/password user, then invoked `https://fdlozdsycfeaxglwstjx.supabase.co/functions/v1/m0-fixture` with the token.
- Observed response: HTTP `200`, body `{"fixture":"m0","version":1,"ok":true}`.
- **Proven:** destination Edge Function executes correctly when invoked by the restored authenticated user.
- **Not proven:** source export from independent backup, clean-target deployment, missing-function recovery, dependency/import map preservation, configuration/secrets recovery, and repeatable redeployment. Function already existed on destination; therefore do not mark the Edge Function restore acceptance criterion complete.

## Capability matrix
| Component | Supabase Cloud | Self-hosted | Proof required |
|---|---|---|---|
| PostgreSQL | Logical dump and managed backups/PITR are distinct | pg_dump/pg_restore; physical backup requires separate validation | Isolated schema/data/roles/RLS/sequence restore |
| Storage | Database backup does not include object bytes | Depends on object backend and bucket config | Paginated export, byte SHA-256 comparison, bucket settings |
| Auth | Auth records can be in DB; external provider settings/secrets separate | Versioned GoTrue configuration and provider secrets | Sign-in to restored target using synthetic users |
| Edge Functions | Source should come from pinned Git/deployment pipeline | Source and runtime configuration topology-specific | Rebuild and invoke clean-target function |
| Settings | Project clone is database-only; manual configuration remains | Docker/Kubernetes and service settings must be inventoried | Inventory comparison and manual recovery |
| Secrets | Secret values may not be readable back; escrow/rotate | External secrets and host environment separately managed | Recovery from independent escrow, never export secrets to Git |

No component can be called "automatically recoverable" until its actual clean-target restore is demonstrated. Logical dumps are not incremental backups or PITR. Cross-service consistency is not atomic.

## Evidence sources
- https://supabase.com/docs/guides/platform/backups
- https://supabase.com/docs/guides/platform/clone-project
- https://supabase.com/docs/guides/platform/migrating-within-supabase/backup-restore
- https://supabase.com/docs/guides/storage/management/download-objects
- https://supabase.com/docs/guides/auth/managing-user-data
- https://supabase.com/docs/guides/self-hosting/self-hosted-functions
- https://supabase.com/changelog.md

## Gate T-01 and M0 acceptance
- [x] Full specification, architecture and test plan in repository
- [x] Cloud and self-hosted provisional six-component capability matrix
- [x] Threat model in `docs/spec/v1.0.0/architecture/ARCHITECTURE.md`
- [x] Isolated experiment protocol and explicit evidence checklist
- [x] Dedicated disposable Cloud source and independent restore target approved
- [ ] Disposable pinned self-hosted source and independent restore target approved
- [ ] CLI, PostgreSQL, Auth, Storage and Functions versions recorded
- [ ] Permissions, TLS, least privilege and cross-project access denial tested
- [ ] DB export/restore with FK, roles, RLS, sequences and fixture hashes (fixture schema, FK, RLS and data checksums passed; roles/grants and sequence behaviour remain)
- [ ] Storage export/restore with pagination and every object checksum
- [ ] Auth records/config restored and synthetic login succeeds (synthetic email/password records and login passed; full Auth config and other providers remain)
- [ ] Edge Function source redeployed and invoked (authenticated invocation of pre-existing destination function passed; independent clean-target redeployment remains)
- [ ] Settings/secrets manual steps executed from independent recovery materials
- [ ] Recovery time, recovery point and component consistency window measured
- [ ] Corrupt backup, missing object and denied privilege tests fail closed
- [ ] Recovery on a fresh host without original Keychain or catalogue
- [ ] Owner and security reviewer sign off documented go/no-go

**Gate decision: NO-GO.** M0 is not complete until both topologies have independently verified restore evidence. No production readiness claims.

## Risks
R-M0-01: Cloud settings and secrets may be non-exportable (high).
R-M0-02: Auth users alone do not guarantee sign-in (high).
R-M0-03: Database and Storage capture are not atomic (high).
R-M0-04: Self-hosted Storage backends and deployment topology vary (high).
R-M0-05: RPO/RTO are provisional until measured (high).

## Sign-off
Test environments: disposable Cloud source and restore projects listed above; self-hosted pending. Evidence location: operator Terminal transcript and local dump. Owner: pending. Security reviewer: pending. Decision: NO-GO.

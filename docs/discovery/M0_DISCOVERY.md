# M0 technical discovery - evidence-led assessment
Date: 3 November 2026. **Status: NOT COMPLETE / NO-GO.** Isolated Cloud PostgreSQL fixture, manual and automated four-object Storage recovery, synthetic email/password Auth recovery, and single Edge Function cross-project redeployment drills have passed. Full Cloud and self-hosted recovery are not yet demonstrated; no production project was modified.

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
- Limits: this is a schema-scoped fixture restore, not full project or database recovery; database roles/grants, full Auth configuration, automated bulk Storage object recovery, independently escrowed Edge Function source, project settings, secrets, fresh-host recovery, sequence increment behaviour, negative tests, RPO/RTO and consistency remain unverified. MD5 comparisons cover selected fixture fields, not all metadata or bytes.

**Next drill:** verify private Storage access denial and automated paginated enumeration; separately prove Edge Function recovery from independently retained source after source loss. Keep real user data and credentials out of evidence and Git.

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

## Executed Cloud Storage three-file manual recovery - 3 November 2026

**Result: PASS based on operator-confirmed byte comparisons for three synthetic objects; automated paginated bulk recovery remains unverified.**

- Source: disposable Cloud project `qfcowkrbothbseswosjg`; independent destination: `fdlozdsycfeaxglwstjx`; private bucket `m0-backup-test` in both.
- Three locally created test objects uploaded to the source and verified by source SQL: `documents/test.txt` (28 bytes, `text/plain`), `images/binary-test.bin` (65,536 bytes, `application/macbinary`), and `nested/deep/data.json` (29 bytes, `application/json`).
- Operator downloaded the source objects into `$HOME/supabase-m0/storage-backup/` preserving relative paths. Local backup SHA-256 checksums matched original fixture files:
  - `documents/test.txt`: `90f2a691a1022eeeebf48033e9f672628a768dc899162ccf8c19527db5b00cb6`
  - `images/binary-test.bin`: `050181748ac0277fd066425225ebb1cfa7d5d1a8e0f8fc10ac3dc0fdec654dc1`
  - `nested/deep/data.json`: `029be1f5767115bb36b045d67d4492e48a75f01f1f3c1bbb5bd3cd11eb103a63`
- An earlier `recovery-test.txt` and macOS `.DS_Store` files were also present locally but were excluded from this three-file drill.
- Operator manually uploaded the three backed-up objects to the independent destination with matching relative paths. Destination SQL returned the expected names, sizes and MIME types for all three.
- Operator was instructed to download the three destination objects independently and compare each to the original fixture using `cmp -s`. Operator confirmed "all okay"; individual final command output was not supplied. Treat this as **operator-attested PASS**, not independently captured checksum evidence for the destination downloads.
- **Proven within this manual drill:** multiple object types, nested folder paths, source backup checksum integrity, destination metadata equality and operator-attested byte equality.
- **Not proven:** automated/paginated object listing, automated bulk export/restore, completeness for large buckets, object versioning, retries/resume, policy equivalence, anonymous access denial, and independent recorded destination SHA-256 values.

## Executed Cloud Storage unauthenticated access checks - 3 November 2026

**Result: PASS for narrowly scoped unauthenticated download denial; full authorisation remains unverified.**

- Destination disposable project `fdlozdsycfeaxglwstjx`, private bucket `m0-backup-test`, object `documents/test.txt`.
- Unauthenticated GET to public object endpoint returned HTTP 400; no object served. Public-endpoint response body not captured.
- Unauthenticated GET to authenticated object endpoint returned HTTP 400 with JSON `{"statusCode":"400","error":"Error","message":"headers must have required property 'authorization'","code":"InvalidRequest"}`, explicitly confirming rejection for missing authorisation header.
- Still untested: valid but unauthorised identity, per-object RLS, source/destination policy equivalence, and authorised download.

## Executed Cloud Storage authenticated access restriction - 3 November 2026

**Result: PASS for a narrowly scoped valid-user denied-read test; policy backup and restore remain unverified.**

- Destination synthetic account `m0-test@example.com` successfully obtained an access token via password authentication. No token or password recorded.
- Destination `storage.objects` had no policies according to `pg_policies`. SQL confirmed RLS enabled (`relrowsecurity=true`) and force RLS disabled (`relforcerowsecurity=false`).
- Authenticated request for `m0-backup-test/documents/test.txt` did not return file content. First status-only invocation reported HTTP 400; a subsequent body-inspection request returned JSON `{"statusCode":"404","error":"not_found","message":"Object not found","code":"NoSuchKey"}`. This 404 is consistent with an RLS-filtered object, given prior independent SQL evidence that the object exists.
- **Limitations:** status-only and body-inspection responses differ (400 vs body status 404); record both, do not assert they were identical. No affirmative authorised-read test or restored policy comparison was performed. Service-role bypass and cross-project isolation not tested here.

## Executed Cloud Storage policy restore and enforcement - 3 November 2026

**Result: PASS for manually reconstructed single-policy recovery and narrow allow/deny enforcement; automated policy export/restore remains unverified.**

- In disposable source `qfcowkrbothbseswosjg`, created `m0_allow_test_user_document` on `storage.objects`: permissive `SELECT` to `authenticated`, limited to bucket `m0-backup-test`, object `documents/test.txt`, and synthetic user UUID `a53144f6-ee4a-41ea-9aa4-259d7ef49a18`.
- Operator verified source policy through `pg_policies`, then manually reconstructed and retained SQL at `$HOME/supabase-m0/backups/storage/m0_storage_policy.sql`, SHA-256 `955c9e779237c6eaffd9952c3755781fc2817220742c0f943d59eb35b9b46adc`. This was **not an automated policy export**.
- Destination `fdlozdsycfeaxglwstjx` initially had no `storage.objects` policies. The policy was created there, and destination `pg_policies` output matched source for policy name, permissiveness, roles, command, qualifier and null `with_check`.
- Using the restored synthetic email/password user, GET `documents/test.txt` returned HTTP **200**, proving permitted read.
- Same user's GET `images/binary-test.bin` returned HTTP **400** in status-only test; subsequent response-body inspection returned JSON `{"statusCode":"404","error":"not_found","message":"Object not found","code":"NoSuchKey"}`. Earlier destination SQL confirmed this object exists. The 404 is consistent with RLS-filtered access; note differing reported statuses between invocations.
- **Scope:** one synthetic user, one policy, one permitted object, one denied object. Not proof of automated extraction, all policies, cross-user isolation, other verbs, service-role behaviour, policy portability or complete Storage recovery.

## Executed controlled interrupted Storage restore and resume - 4 November 2026

**Result: PASS for a deliberate two-object interruption, safe resume of two missing objects, and independently verified final destination bytes. Overall M0 remains NO-GO.**

- Disposable destination project `fdlozdsycfeaxglwstjx`; new private bucket `m0-interrupted-restore-test`, verified private and initially empty.
- Operator verified all four source-backup objects against the SHA-256/size manifest before any uploads. Uploaded `documents/test.txt` and `images/binary-test.bin` using `x-upsert: false`, then deliberately stopped before the other two objects. Output: `SIMULATED INTERRUPTION: Restore stopped after two objects`.
- Independent read-only inspection downloaded and verified both existing objects. Requests for `nested/deep/data.json` and `recovery-test.txt` returned HTTP 400 with Storage API code `NoSuchKey` and message `Object not found`. The initial checker incorrectly treated only HTTP 404 as missing; a follow-up diagnostic established the actual API error semantics.
- Resume script verified all four local files first; independently downloaded and checksum-verified the two existing destination objects, and treated only explicit `NoSuchKey` (HTTP 400/404) as missing. Unexpected errors or differing existing bytes caused refusal. It required exactly two existing and two missing objects before writing.
- Output: `SKIP VERIFIED` for the two existing objects; `MISSING` and then `RESTORED` for the remaining two; `SUCCESS: Interrupted restore resumed`, `Existing verified objects preserved: 2`, `Missing objects uploaded: 2`.
- Independent post-resume authenticated downloads and SHA-256/size comparisons: `PASS` for all four objects, `Verified: 4/4`, `SUCCESS: Interrupted restore fully recovered`.
- **Limits:** this was a deliberately stopped upload sequence with manual invocation of an ad hoc resume script, not a process crash or actual network interruption. No automated retries/backoff, crash-safe persisted state, concurrency/race test, large-bucket completeness, exact destination enumeration or overwrite-conflict injection was demonstrated. Existing objects were verified and skipped, not overwritten. No production data or projects were involved.

## Executed Storage corruption detection and restore refusal - 3 November 2026

**Result: PASS for local checksum corruption detection and isolated restore refusal; no network requests in restore refusal test.**

- Copied the four-object backup to `$HOME/supabase-m0/tests/corrupted-backup` without modifying the original verified backup.
- Appended `SIMULATED CORRUPTION\n` to `objects/documents/test.txt` while preserving the original manifest.
- Independent local integrity checker identified exactly one corrupted object (`documents/test.txt`); the remaining three passed SHA-256 and size verification. Operator output: `Corrupted files detected: 1`, `TEST PASSED: Corrupted backup identified`.
- Created an isolated copy of `restore_storage.py` pointing to corrupted backup. Stubbed the destination private-bucket metadata response, with all other API calls replaced by a network-disabled exception.
- Operator output: `PASS: Mock destination bucket configured`, `ERROR: Backup integrity mismatch: documents/test.txt`. This confirms refusal before the script's upload stage. No destination mutation and no network access in the isolated refusal test.
- **Limitations:** this was a simulated network-free test, not a real interrupted restore; the script currently checks destination bucket metadata before local file integrity. Retry, partial upload recovery, timeout, and resume behaviour remain untested.

## Executed Cloud Storage automated backup and restore - 3 November 2026

**Result: PASS for four-object automated API-based backup, restore to empty private bucket, and independently verified destination bytes. Overall M0 remains NO-GO.**

- Source disposable project `qfcowkrbothbseswosjg`, private bucket `m0-backup-test`; destination disposable project `fdlozdsycfeaxglwstjx`, newly created private bucket `m0-automated-restore-test`.
- Operator's local macOS environment: Python 3.9.6 and jq 1.7.1. Source and destination project-specific secret keys supplied to separate Terminal session variables, not stored in evidence or Git.
- `$HOME/supabase-m0/scripts/backup_storage.py` used Supabase Storage API with page size 2, offset-based listing, recursive traversal of virtual folders (`id=null`), authenticated object downloads, expected-size checks, and local SHA-256 manifest at `$HOME/supabase-m0/backups/storage/automated/manifest.json`.
- Root listing observed offset 0: 2 entries; offset 2: 2 entries; offset 4: empty. Nested folder listings returned expected entries. Four objects downloaded: `documents/test.txt` (28 bytes), `images/binary-test.bin` (65,536 bytes), `nested/deep/data.json` (29 bytes), `recovery-test.txt` (453 bytes).
- Operator independently checked all four local backup files against manifest SHA-256 and size: 4/4 PASS.
- `$HOME/supabase-m0/scripts/restore_storage.py` validated fixed disposable project/bucket IDs, source manifest identity, all four local file checksums and sizes, private destination bucket status and empty root listing before uploading four objects with `x-upsert: false`. Operator output: four `RESTORED` lines and `SUCCESS: Uploaded 4 objects`.
- Independent destination API download script fetched each restored object using destination credentials and compared SHA-256 and byte size against original manifest: **4/4 PASS**, `SUCCESS: All restored objects match the backup manifest`.
- **Scope and limitations:** four synthetic objects; pagination demonstrated at root but not stress-tested at scale; restore is a one-off local Python script, not integrated into the backup manager; no retries/resume, failure injection, deletion reconciliation, bucket configuration or policy restoration within automated workflow, service-role hardening, independently escrowed credentials, cross-service consistency, measured RPO/RTO or source-project loss scenario. Backup scripts and artefacts remain on operator's Mac, not committed to Git. No production data used.

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

## Executed Cloud Edge Function cross-project recovery - 3 November 2026

**Result: PASS for recovery and authenticated execution of one synthetic Edge Function absent from the destination before deployment.**

- Verified that `m0-edge-recovery` was absent from the independent destination `fdlozdsycfeaxglwstjx`; created and deployed it only in disposable source `qfcowkrbothbseswosjg` with `verify_jwt = true`.
- Retrieved source `index.ts` from the active source deployment using Supabase management tooling, then redeployed that retrieved source to the destination as new function `m0-edge-recovery` with `verify_jwt = true`.
- Inspected destination deployment: status ACTIVE, version 1, retrieved `index.ts` content matched the source; function-specific deployment bundle hashes differed and are not treated as byte-identical artifacts.
- First operator invocation attempt failed at Auth token retrieval with HTTP `401` due to a key-entry issue; function was not invoked on that attempt. Following correction, operator obtained an authenticated destination token and invoked the recovered function.
- Successful invocation: HTTP `200`, JSON body `{"fixture":"m0-edge-recovery","version":1,"restorable":true}`.
- **Proven:** source extraction from an existing deployed function, cross-project redeployment into a destination previously lacking that function, and successful invocation using the recovered synthetic Auth account.
- **Not proven:** independent offline backup escrow or Git-pinned source availability after source-project loss; multi-file dependencies, import maps, secret/environment configuration, external integrations, automation, or complete project-level Edge Function recovery. Source was retrieved while the source project was available.

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
- [ ] Permissions, TLS, least privilege and cross-project access denial tested (unauthenticated requests rejected; one restored policy permits intended document and restricts another object; cross-project denial and broader privilege checks remain)
- [ ] DB export/restore with FK, roles, RLS, sequences and fixture hashes (fixture schema, FK, RLS and data checksums passed; roles/grants and sequence behaviour remain)
- [x] Storage export/restore with pagination and every object checksum (four synthetic objects automated, root pagination and nested folders tested, destination bytes independently verified; larger scale and production readiness not established)
- [ ] Auth records/config restored and synthetic login succeeds (synthetic email/password records and login passed; full Auth config and other providers remain)
- [x] Edge Function source redeployed and invoked (one synthetic function deployed to previously absent destination and authenticated HTTP 200 verified; independent offline source escrow, dependencies and secrets remain unverified)
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

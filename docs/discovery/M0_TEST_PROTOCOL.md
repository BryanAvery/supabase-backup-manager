# M0 disposable recovery test protocol

Never run destructive tests against production or existing personal projects. Obtain explicit approval for synthetic source and separate empty target for **each** of Cloud and self-hosted.

1. Record project/host IDs privately, versions, owner, test date, target-is-empty proof and fixture hash. No credentials or private data in GitHub.
2. Inspect CLI help before executing commands: `supabase --help`, `supabase db --help`, `supabase storage --help`. Record CLI, pg_dump, pg_restore, PostgreSQL and service versions.
3. Prepare synthetic PostgreSQL schema with foreign keys, sequences, RLS, role grants, triggers, views and expected row hashes. Include enough Storage objects for multiple listing pages, binary/empty objects and bucket settings. Prepare synthetic Auth accounts and Git-pinned function.
4. Record API/DB permissions and explicit denials for insufficient privilege and cross-project access.
5. Export database with compatible logical tooling; restore to **new** target; verify schema, data, RLS, roles, FK, sequence values and row hashes.
6. Export Storage bytes **separately**; compare source and target object SHA-256, counts, metadata and bucket configuration. Record source change window.
7. Restore Auth and provider settings through supported versioned procedure. Prove synthetic user sign-in and access boundaries.
8. Redeploy function from pinned source, provision escrowed synthetic secrets, reapply configuration and smoke-test invocation.
9. Exercise invalid manifest, corrupt object, expired credentials, interrupted upload and missing permissions; verify fail-closed behaviour.
10. Recover using separate host without original Mac, Keychain or SQLite catalogue. Record elapsed restore time and achievable RPO.
11. Repeat all steps independently on Cloud and self-hosted; do not infer support from one to the other.
12. Record outcome per component as captured, manual, unsupported or failed. Manual/unsupported require documented limitation and sign-off.

## Evidence record (one per topology)
Fields: test_id=T-01, environment, source/target versions, CLI version, synthetic fixture SHA-256, artefact SHA-256, operator, timestamps, per-component result and restore evidence hash, permissions tests, negative tests, measured RPO/RTO, limitations, owner/security decision. Store evidence in restricted immutable storage, not in this public repository.

M0 passes only with two approved environment records, all six components assessed, actual recovery proof for claimed support, explicit manual gaps, negative security tests, measured timings and signed go/no-go.

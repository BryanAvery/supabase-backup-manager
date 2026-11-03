# M0 technical discovery - evidence-led assessment
Date: 3 November 2026. **Status: NOT COMPLETE / NO-GO.** Desk research is complete; no disposable Cloud or self-hosted restore drills have been run. No existing user project was modified.

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
- [ ] Dedicated disposable Cloud source and independent restore target approved
- [ ] Disposable pinned self-hosted source and independent restore target approved
- [ ] CLI, PostgreSQL, Auth, Storage and Functions versions recorded
- [ ] Permissions, TLS, least privilege and cross-project access denial tested
- [ ] DB export/restore with FK, roles, RLS, sequences and fixture hashes
- [ ] Storage export/restore with pagination and every object checksum
- [ ] Auth records/config restored and synthetic login succeeds
- [ ] Edge Function source redeployed and invoked
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
Test environments: pending. Evidence location: pending. Owner: pending. Security reviewer: pending. Decision: NO-GO.

# Release plan and acceptance gates

## Release strategy
**Preview releases** are engineering increments; **R1 production** includes complete *supported* recovery coverage across all six components. No cloud or self-hosted capability is assumed without validation.

| Release | Scope | Entry criteria | Exit / acceptance gate |
|---|---|---|---|
| M0 discovery | API/permission matrix, topology matrix, threat model, gap register | Approved test projects | T-01 evidence; explicit source/secret limitations and go/no-go |
| M1 foundation | Go service, SQLite, launchd, SwiftUI and web shells, RBAC/API | M0 architecture decisions | Authenticated UI/API, migration rollback and project isolation T-13 |
| M2 core capture | DB, Storage, Auth, scheduler, manifests | M1 | T-02/T-03/T-04/T-12 in isolated lab; no false complete status |
| M3 functions/settings | Git source, settings, secrets escrow references | M2 | T-05/T-06; gaps visible and runbooks tested |
| M4 repositories | Five destinations, AEAD, independent key recovery | M2 | T-09/T-10/T-11, tamper detection and partial replica handling |
| M5 guided recovery | Full and partial restore, approvals, cutover plan | M3/M4 | T-07/T-08 and Auth/function/settings smoke tests |
| M6 hardening | Security, accessibility, scale, packaging, upgrade | M5 | T-13/T-14/T-15/T-16; readiness review |
| **R1.0 production** | All above integrated | M6 | **All P0 requirements tested**, Cloud+self-hosted full drills, independent host recovery, approved gaps and runbooks |

## Gate checklist
- [ ] Requirement-to-test traceability complete; no P0 `not_run`.
- [ ] All six component categories assessed for both deployment models.
- [ ] Full restore drill includes Storage bytes, Auth login and Edge Function invocation.
- [ ] Project settings and external secrets manual recovery steps documented and rehearsed.
- [ ] Recovery set consistency window and gaps reported accurately.
- [ ] Five storage adapters independently validated.
- [ ] Host-loss recovery without original Mac and Keychain demonstrated.
- [ ] Signed/notarised macOS package, reproducible builds, SBOM and dependency scanning.
- [ ] Measured RPO/RTO, on-call alerts, operational runbooks and recovery-key escrow sign-off.
- [ ] No unresolved critical security defects; release approval recorded with SHA.

## Backlog prioritisation
P0 recovery/security obligations are release blocking. P1 UI/performance/product obligations are targeted for R1 unless explicitly rescheduled with owner approval. P2 enhancements (PITR integrations, efficient incremental capture, richer diff) follow after R1 only when not required for a project's declared RPO.

## Change management
Each issue links to requirement IDs and test IDs. Every release candidate publishes requirements delta, API diff, migration plan, risk register, passed/failed tests, known limitations, build hash and sign-offs.

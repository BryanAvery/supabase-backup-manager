# M1 - Platform Foundation: Planning Baseline

**Status:** Draft for review - not approved for implementation or release  
**Prepared:** 4 November 2026  
**Repository:** `BryanAvery/supabase-backup-manager`  
**Branch:** `planning/m1-platform-foundation`  
**Milestone:** M1 (M0 discovery -> M1 foundation -> M2 core capture)

## 1. Purpose

Build the secure, independently running application foundation that the M2-M6 backup and recovery features will use. M1 is **not** a functional backup product, and neither UI may suggest that a backup or restore has completed when it has not.

The normative scope is in:
- [Release plan](../spec/v1.0.0/releases/RELEASE_PLAN.md)
- [SRS](../spec/v1.0.0/requirements/SRS.md)
- [Architecture](../spec/v1.0.0/architecture/ARCHITECTURE.md)
- [Initial SQLite schema](../spec/v1.0.0/data/schema.sql) and [data model](../spec/v1.0.0/data/MODEL.md)
- [OpenAPI 3.1 contract](../spec/v1.0.0/api/openapi.yaml)
- [Release gates](../testing/RELEASE_GATES.md)

The primary target is an Apple Silicon Mac mini. A Go service is the only privileged management/backend process. The native SwiftUI client and React/TypeScript browser dashboard are clients of the same versioned authenticated API. SQLite stores **metadata and secret references only**, not credentials, backup data or encryption keys. Initial networking is local-only; remote access must be explicitly enabled using a private network/overlay.

## 2. M0 handover and prerequisites

M0's disposable Supabase Cloud recovery fixtures and two-project identity protection supply useful evidence for M2 design; they are **not** part of the M1 runtime. Keep the separate source and disposable recovery target, and never point M1 prototype tests at the live SwimTayka project.

Before M1 implementation is considered release-gate ready:
- [ ] Attach and review the latest successful M0 GitHub Actions evidence (both cycles, ACLs/RLS, integrity, source/target identity checks); record run ID, SHA and exact checks passed.
- [ ] Maintain a clear distinction between **fixture-level Cloud validation** and the original full M0 requirement (Cloud/self-hosted capability matrix, unexportable managed settings, documented limitations and approved go/no-go).
- [ ] Preserve open M0 items as explicit risks. M1 planning can progress without silently marking M0 complete.
- [ ] Reconfirm the signed-off M0 architecture choices and their consequences for daemon identity, permissions and recovery keys.
- [ ] Use synthetic project records and test identity providers for M1; no production Supabase credentials or destructive operations.

## 3. Work packages and suggested implementation order

| ID | Work package | Key deliverables | Exit check / evidence | Depends on |
| --- | --- | --- | --- | --- |
| M1-01 | Repository and CI scaffold | Go module, macOS app target, React/TypeScript app, build/test commands, dependency pinning and CI checks | Clean checkout builds/tests and records toolchain versions | Decisions below |
| M1-02 | SQLite catalogue and migrations | Transactional migration runner using existing schema as baseline, WAL/foreign keys/integrity checks, safe migrations, backup + rollback procedure | Fresh install, repeat apply, failed migration, migration back/forward on copy, relational constraints | M1-01 |
| M1-03 | Go service and API | Versioned `/api/v1` routing, health, authenticated project list/create within agreed scope, structured redacted logs, graceful shutdown, bounded resource use | API contract and integration tests, no unauthenticated data access | M1-01, M1-02 |
| M1-04 | Identity, sessions and RBAC | Agreed identity provider, secure login/logout, server-side project/role enforcement, audit records, CSRF/origin protection, request limits | T-13 M1 security subset including denied cross-project reads/writes and session expiry | M1-02, M1-03 |
| M1-05 | Native macOS shell | SwiftUI sign-in/session, permitted project list and project status, API client generated/validated from OpenAPI | Native client connects securely, handles expired session/access denied | M1-03, M1-04 |
| M1-06 | Web shell | React/TypeScript sign-in/session, responsive project list/status, generated/validated API client | Web client connects securely on local/private route only, displays denied access correctly | M1-03, M1-04 |
| M1-07 | launchd packaging and operations | Separate non-privileged service identity, launchd service definition, install/uninstall/upgrade procedure, logs/health, service restart | Service stays up when UI exits; Mac reboot and daemon crash tests on Apple Silicon | M1-03, M1-04 |
| M1-08 | Milestone security/reliability acceptance | M1 test matrix, evidence bundle, architecture/security review, supported configurations, issue register | All M1 exit checks evidenced; explicit deferrals and sign-off | M1-01 to M1-07 |

**PR slicing:** one small PR per work package where practical; add fixtures, unit/negative-path tests and documentation with the implementation. Keep the M0 test harness isolated. Avoid one large PR combining identity, service installation and both UIs.

## 4. First usable M1 demonstration

1. After Mac reboot, the Go daemon starts under launchd without opening either UI or logging into the GUI.
2. An authorised user signs into the SwiftUI app and sees only projects they may access.
3. The same user signs into the web UI over a permitted interface and sees the same project-scoped data.
4. An unauthenticated or expired session is denied; a second user cannot read or alter another project's records by guessing an ID or calling the API directly.
5. A cancelled or failed database migration does not corrupt or partially advance the catalogue; the documented rollback path works on a copy.
6. Health/audit/log output is diagnostic but contains no credentials or personal test secrets.
7. Stopping or closing either UI has no effect on daemon availability. Unsupported backup/restore controls are hidden or clearly disabled.

## 5. Security boundaries to hold from the first commit

- **Default bind:** loopback only; no port publicly reachable or exposed by default. Decide private-overlay/Tailscale-style remote access design before enabling it.
- **Authentication:** reject anonymous management endpoints except narrowly scoped health readiness as defined in the API. Agree OIDC/passkey/MFA approach, native-app token handling and browser cookie/CSRF approach before coding.
- **Authorisation:** enforce server-side on every endpoint and on project IDs, including future SSE connections. UI visibility is not a security control.
- **Roles:** Viewer, Operator, Administrator and Recovery Approver are scoped per project. A Recovery Approver cannot self-approve destructive requests when dual approval is enabled; full destructive restore flows belong to M5/M6.
- **Storage:** SQLite contains opaque `secret_ref` entries, never Supabase database URLs containing passwords, cloud credentials, raw tokens or backup payloads.
- **Keychain:** prove unattended access under the chosen launchd service identity; do not assume a logged-in user's Keychain will be available after reboot.
- **Audit:** record actor, project, action, outcome, request ID and time without secrets. Negative authorisation decisions must be testable.
- **Build hygiene:** pin dependencies, enable secret/dependency scans and keep sensitive fixtures out of Git.

## 6. Architectural decisions to resolve before M1-01/M1-04

| Decision | Proposed starting point (subject to approval) | Why it matters |
| --- | --- | --- |
| Minimum macOS version and build machines | Record a supported Apple Silicon/macOS matrix and a real Mac for launchd/SwiftUI tests | CI on Linux cannot verify reboot, installer or Keychain behaviour |
| Identity provider | Standards-based OIDC with MFA available; synthetic identities in tests | Avoid inventing a second insecure credential store |
| Local vs remote connection | Loopback by default; explicit private-overlay access with TLS | Browser/mobile access must not expose a public management API |
| Service account and file permissions | Dedicated least-privilege service identity and protected catalogue path | Prevent GUI session dependence and credential leakage |
| Keychain/external vault integration | Test non-interactive access and keep only references in SQLite | `launchd` startup and disaster-recovery design must remain viable |
| API contract increments | Use existing OpenAPI 3.1; expose a strictly minimal M1 endpoint subset and document missing routes | No client/backend drift or fake implementation of M2 endpoints |
| Rollback semantics | Transactional, ordered schema migrations plus catalogue snapshot and tested restore | SQLite schema migrations are not all inherently reversible |
| Remote access identity mapping | Explicit per-project membership and deny-by-default provisioning | Multi-user isolation must survive direct API requests |

Record approved changes as ADRs. The existing ADR-0001 is proposed, not a final sign-off.

## 7. M1 test and acceptance matrix

**Test scope:** M1 subset of T-13 (access controls), plus applicable T-12/T-16 platform behaviours. Full T-13 (destructive step-up approval), T-12 scheduling/failure injection and T-16 notarised releases remain part of later milestones; do **not** mark those entire test IDs passed solely from M1 work.

| Evidence ID | Scenario | Pass condition |
| --- | --- | --- |
| M1-A01 | First install and service boot | Go service starts; health and API version are discoverable locally |
| M1-A02 | UI closed / logged-out Mac / reboot | Service runs without either UI and survives supported restart sequence |
| M1-A03 | SQLite migration 0->N, rerun, fail mid-flight, rollback on copy | Consistent schema and `integrity_check`, foreign keys enforced, no partial migrations |
| M1-A04 | Valid login, logout, expired/revoked session | Session life-cycle works; stale sessions and protected API access denied |
| M1-A05 | Cross-project data reads/writes and ID enumeration | Same user with access to project A cannot see or change project B; fail closed |
| M1-A06 | Viewer/operator/admin/approver permissions | Role matrix enforced by backend, not UI-only |
| M1-A07 | Browser CSRF/origin and API request throttling | Cross-origin state changes and abusive requests rejected |
| M1-A08 | Local/private network exposure | No unauthorised public listener; documented, tested remote access path only |
| M1-A09 | SwiftUI and React both use API contract | Secure successful sign-in and permitted project list/status; error and retry states correct |
| M1-A10 | Service restart / catalogue error / redaction | Safe restart and recoverable errors; no misleading success, credentials or secrets in logs |
| M1-A11 | Clean checkout CI and version provenance | Builds/tests validate, toolchain versions and artefact SHA recorded |

**Evidence format:** run date, commit SHA, Mac and OS versions, test identity/fixture reference, commands, pass/fail, sanitised logs, reviewer and open defects. Test against a real Apple Silicon machine for A02/A08/A09. No M1 requirement is accepted based only on a UI screenshot or simulated success.

## 8. Milestone gates

**Entry to implementation**
- [ ] M1 architecture, minimum platform and identity choices recorded.
- [ ] M0 handover evidence linked and open limitations tracked.
- [ ] The initial OpenAPI routes and SQLite schema changes (if any) reviewed.
- [ ] Security model and per-project RBAC matrix reviewed.
- [ ] Build/test responsibilities and real macOS test environment identified.

**Exit from M1**
- [ ] Go daemon runs under launchd independently of UI windows and after reboot.
- [ ] Both SwiftUI and web interfaces use authenticated and authorised versioned API endpoints.
- [ ] Project isolation, session expiry, denied access, CSRF/origins and rate limiting have negative tests.
- [ ] SQLite migration, failure, integrity, backup and rollback-on-copy tests pass.
- [ ] Local/private binding and safe operational logs are verified.
- [ ] Relevant M1-Axx evidence and requirement mappings are linked to accepted PRs.
- [ ] Risks, known gaps, decisions and deferrals are published; gate approved before M2.

## 9. Explicitly outside M1

Actual PostgreSQL/Storage/Auth capture, scheduling and recovery-set publication are M2. Edge Function/settings recovery is M3; encrypted multi-cloud replicas and independent key recovery are M4; guided production restore is M5; production packaging, notarisation, accessibility, scale and complete security hardening are M6. M1 may include seams/interfaces for these functions but must not claim to provide them.

## 10. Planning notes and immediate next actions

1. Review this baseline and record decisions in section 6.
2. Turn work packages M1-01 to M1-08 into implementation issues with stable requirements, expected tests, owner and dependencies.
3. Start M1-01 (scaffold/CI) and M1-02 (catalogue migrations) in small, reviewable feature branches after planning approval.
4. Keep M0 evidence verification and outstanding full discovery requirements tracked separately; do not weaken its gate to advance M1.

# Supabase Backup Manager
## Software Requirements Specification (SRS)

**Version:** 1.0  
**Date:** 3 November 2026  
**Status:** Baseline for design and implementation; subject to technical discovery and validation  
**Target platform:** Apple Silicon macOS  
**Deployment:** Self-hosted backup service with native macOS app and secure responsive web dashboard  
**Environments:** Supabase Cloud and self-hosted Supabase  
**User model:** Multi-user, role-based access control  
**Recovery:** Guided with explicit approvals  
**Release priority:** Complete supported Supabase recovery coverage before production release

## 1. Purpose, scope and governing principles

The product protects Supabase projects against accidental deletion, corruption, credential loss, infrastructure failure, malicious modification and loss of the backup host. It captures recoverable PostgreSQL database content, Storage objects and metadata, Auth records and configuration, Edge Functions, project configuration, and recoverable secrets or their recovery procedures. It must support both selective recovery and reconstruction of a replacement project.

**Important scope boundary:** 'Complete protection' means every in-scope component has a verified automated backup/restore method **or** a documented, tested manual recovery procedure and explicitly disclosed gap. It does not imply Supabase-managed secrets, signing material, external identity-provider configuration or infrastructure can always be exported. A project must never be marked fully automated/recoverable when such gaps remain.

**Principles:** recovery-first design; least privilege; encryption by default; independent recovery from another machine; no unverified success claims; explicit consistency windows; reversible operations where feasible; provider-specific capability discovery; no public exposure of management ports by default.

### 1.1 Stakeholders and actors
- Platform owner: establishes global policy, storage and encryption controls.
- Administrator: manages projects, access, scheduling and approved restores.
- Operator: initiates backups, reviews failures and performs permitted recovery steps.
- Auditor: reads evidence, logs, coverage and reports without changing data.
- Recovery approver: independently approves high-risk production operations when two-person control is enabled.
- Backup service: executes unattended jobs and retention operations.

### 1.2 Definitions
- **Recovery set:** signed, versioned manifest referencing separately encrypted component archives and their capture times.
- **RPO:** maximum acceptable data loss measured against the latest usable recovery point.
- **RTO:** target time to restore service, measured in rehearsals.
- **Verified:** archive checksum/authenticity and structural checks passed; not synonymous with a successful restore.
- **Restore-tested:** actual recovery into an isolated environment passed defined functional checks.
- **Capability matrix:** per-project assessment of supported exports, imports, privileges and manual dependencies.

### 1.3 Defaults and assumptions
- Initial host: Apple Silicon Mac mini, 16 GB RAM recommended, supported macOS release, separate backup volume and UPS recommended.
- Initial remote access: private network via Tailscale or equivalent; local-only binding until configured.
- Initial cloud destination: AWS S3 plus local archive; Azure Blob, Google Drive and OneDrive mandatory before production release.
- Initial backup policy: daily complete recovery set at 02:00 host local time, weekly restore verification, 30 daily + 12 monthly retention as editable defaults.
- All source database operations read-only unless an explicitly approved restore targets that database.
- Production restore defaults to a new isolated project/database; overwrite requires a fresh safety backup and stronger approval.
- Recovery goals default to **RPO 24 hours / RTO 8 hours** as provisional targets, not guarantees; measured feasibility must be demonstrated per project.
- Offline-capable core: backup execution continues when internet is unavailable, provided source and destination are reachable.

## 2. System context and architecture

```text
 macOS SwiftUI app            Browser / mobile dashboard
          |                           |
          +-------- authenticated API-+
                         |
              Go management service
   +---------------------+--------------------+
   | RBAC / audit / API / scheduler / job queue|
   | recovery orchestrator / capability checks|
   +---------------------+--------------------+
                         |
    +----------+---------+----------+----------+
    |PostgreSQL | Storage | Auth     | Functions| Project config/secrets
    |adapter    | adapter | adapter  | adapter  | adapters
    +----------+---------+----------+----------+
                         |
             encrypted recovery-set store
               /     |      |       |                   Local    S3   Azure   GDrive  OneDrive

 Portable recovery CLI reads manifests/archives without original Mac.
```

The Go daemon runs as a managed macOS launchd service, independently of user sessions and UI windows. SQLite stores the operational catalogue, with transactional migrations, WAL mode, backups and integrity checks. Secrets are kept in macOS Keychain or an explicitly configured external vault/KMS. The API uses an OpenAPI contract with generated Swift and TypeScript clients. Web frontend is React/TypeScript; native frontend SwiftUI. Long-running tasks are queued and progress is streamed via authenticated server-sent events. Provider adapters are versioned and capability-aware.

**Trust boundaries:** browser never receives database credentials, encryption master keys or unrestricted cloud credentials; remote dashboard calls authenticated API only; backup workers receive narrowly scoped, short-lived credentials where feasible. The recovery CLI uses separately escrowed recovery material and cannot assume access to the original Keychain.

## 3. Requirement conventions

Every requirement below is mandatory for production release unless marked **Should**. Priority P0 = release-blocking safety/recovery, P1 = required product function, P2 = enhancement. Acceptance evidence is named after the semicolon as **AC**. All requirements are testable and traceable to section 12 test IDs.


## 4. Project onboarding and capability assessment

| ID | Priority | Requirement | Acceptance criterion |
|---|---|---|---|
| **PRJ-001** | P0 | Register, edit, disable and remove multiple independent Supabase Cloud and self-hosted projects. | Five projects can be configured and operated independently without cross-project data leakage. |
| **PRJ-002** | P0 | Support guided connection using Supabase connection string/project reference and advanced PostgreSQL TLS connection details. | Wizard verifies DNS, TLS hostname, credentials, server version and required permissions. |
| **PRJ-003** | P0 | Discover supported backup/restore capabilities for database, Storage, Auth, Edge Functions, settings and secrets. | Per-component capability matrix records supported, manual, unsupported and unknown with evidence and timestamp. |
| **PRJ-004** | P1 | Support direct PostgreSQL connections and supported session-pooler fallback without assuming transaction pooling is suitable for dumps. | Connection diagnostics distinguish modes and validate actual dump operation. |
| **PRJ-005** | P0 | Protect connection secrets in Keychain/vault and redact logs. | No secret value appears in SQLite, diagnostic bundles or client responses. |
| **PRJ-006** | P1 | Discover schema/table inventory, project version, extensions, storage buckets, functions and exportable settings. | Inventory refresh detects changes and marks incomplete discovery. |
| **PRJ-007** | P1 | Tag projects by environment, owner, criticality and dependent applications. | Reports and schedules filter by tags; dependencies appear in recovery plans. |
| **PRJ-008** | P0 | Isolate tenants/projects in API, catalogue and archive paths. | Authorisation tests prevent project A users accessing project B data. |


## 5. Capture and backup orchestration

| ID | Priority | Requirement | Acceptance criterion |
|---|---|---|---|
| **BKP-001** | P0 | Create coordinated recovery sets with independently versioned component archives and capture timestamps. | Manifest lists all selected components, completeness and capture consistency window. |
| **BKP-002** | P0 | Support PostgreSQL full logical, schema-only, data-only, selected-schema and selected-table exports. | Test restores reproduce expected objects, rows and constraints. |
| **BKP-003** | P0 | Capture recoverable database roles/grants, extensions, triggers, functions, views, sequences and RLS definitions. | Manifest identifies every included/excluded object and privileged-operation limitation. |
| **BKP-004** | P0 | Capture Storage bucket definitions, policies, metadata and actual object bytes; do not confuse metadata with files. | Object counts, bytes and checksums reconcile with enumerated source at capture boundary. |
| **BKP-005** | P0 | Capture permitted Auth users, identities and related records plus exportable Auth configuration without leaking credential material. | Restore drill verifies sign-in scenarios; missing signing/OAuth/SMTP secrets are disclosed. |
| **BKP-006** | P0 | Archive Edge Function source from linked Git/source bundle, deployment manifests and runtime settings where accessible. | Every deployed function maps to a recoverable source revision or is flagged as a blocking gap. |
| **BKP-007** | P0 | Capture available project settings, database/webhook/cron/realtime configuration and external integration inventory. | Recovery report separates automatically restorable settings from manual steps. |
| **BKP-008** | P0 | Capture secret names and references; escrow values only via supported authorised source and separate encryption policy. | No unsupported claim of exporting unreadable secrets; restoration plan identifies regeneration/rotation. |
| **BKP-009** | P1 | Support per-project hourly/daily/weekly/monthly and custom schedules, timezone and DST handling. | Scheduler runs intended local wall-clock policy and documents DST handling. |
| **BKP-010** | P0 | Persist job state, retries, backoff, cancellation and restart reconciliation. | Crash during dump/upload cannot produce completed status; safe retry resumes or restarts. |
| **BKP-011** | P1 | Allow manual on-demand backup and pre-deployment API/CLI trigger. | Authorised trigger creates traceable job with idempotency key. |
| **BKP-012** | P0 | Support backup verification by checksums, signed manifests, archive inspection and periodic actual restore tests. | Corrupt archive is rejected; successful checksum alone is not labelled restore-tested. |
| **BKP-013** | P1 | Capture source versions, client tool versions, permissions, exclusions, warnings, duration and size. | Backup detail report displays provenance and compatibility. |
| **BKP-014** | P1 | Support differential or incremental strategies only where a validated provider-specific mechanism exists. | Unsupported modes are disabled and clearly explained, not simulated with unqualified full dumps. |
| **BKP-015** | P0 | Do not label independent component captures as a single atomic point-in-time snapshot. | Manifest exposes start/end timestamps and cross-component consistency limitations. |


## 6. Storage, retention and archive integrity

| ID | Priority | Requirement | Acceptance criterion |
|---|---|---|---|
| **STO-001** | P0 | Store encrypted backups on local volumes and AWS S3, Azure Blob, Google Drive and OneDrive. | Integration tests upload, list, download, decrypt and verify from each provider. |
| **STO-002** | P0 | Support multiple simultaneous destinations and per-replica status. | Partial upload failure does not mark all replicas successful. |
| **STO-003** | P0 | Encrypt archives client-side using authenticated encryption and unique data keys. | Tampered ciphertext fails verification; no plaintext reaches cloud destination. |
| **STO-004** | P0 | Support independently recoverable master-key escrow and documented key rotation. | Fresh machine recovers an archive without original Mac or Keychain. |
| **STO-005** | P1 | Support resumable/multipart transfers where provider permits and retry with backoff. | Simulated interruption recovers without publishing a partial archive. |
| **STO-006** | P0 | Apply configurable retention by project, policy, backup class and destination. | Expiry dry-run and execution preserve pinned and minimum-good backups. |
| **STO-007** | P0 | Support immutability/legal hold on capable providers, with clearly labelled limitations elsewhere. | Deletion attempts fail during immutable retention; UI does not misrepresent Drive/OneDrive. |
| **STO-008** | P1 | Forecast storage use and detect capacity/quota thresholds. | Warnings fire before configured threshold; graphs reconcile with catalogue. |
| **STO-009** | P0 | Use signed/versioned manifests and content hashes, with atomic publication after all mandatory checks. | Unfinished set is not discoverable as complete. |
| **STO-010** | P0 | Preserve enough catalogue metadata off-host for disaster recovery. | Portable recovery CLI discovers remote archives after total host loss. |


## 7. Recovery and disaster recovery

| ID | Priority | Requirement | Acceptance criterion |
|---|---|---|---|
| **RST-001** | P0 | Provide guided restore for full supported database, schema, table, selected rows and Storage objects/buckets. | Each mode passes isolated-target integration test. |
| **RST-002** | P0 | Provide full-project recovery wizard covering database, Storage, Auth, functions, settings, secrets and dependencies. | Drill rebuilds supported project in clean environment and lists manual actions. |
| **RST-003** | P0 | Default to isolated target; production overwrite requires impact preview, fresh safety backup and explicit confirmation. | Unsafe restore cannot proceed without required gates. |
| **RST-004** | P0 | Stage row-level recovery, preview primary-key changes, foreign keys, triggers, sequences and conflicts. | Preview lists inserts/updates/conflicts; no unrelated row changes. |
| **RST-005** | P0 | Check version compatibility and required privileges before restore. | Unsupported major-version/extension mismatch blocks or generates remediation plan. |
| **RST-006** | P0 | Restore functions from verified source revision and reapply supported settings in dependency order. | Function smoke tests pass; unexportable secret values require guided entry. |
| **RST-007** | P0 | Validate Auth by test accounts/provider flows, not just record counts. | Defined sign-in tests pass or recovery remains conditional. |
| **RST-008** | P0 | Support portable CLI recovery from off-site archives on replacement macOS/Linux host. | Fresh-machine drill succeeds without original service database. |
| **RST-009** | P0 | Generate step-by-step cutover/rollback plan for application environment variables, URLs, DNS and external providers. | Cutover is approval-gated and rollback steps are testable. |
| **RST-010** | P0 | Record measured RPO/RTO, restore logs, verification evidence and unresolved gaps. | Recovery report shows elapsed time, latest usable point and failed checks. |
| **RST-011** | P1 | Support selective restore into new table/schema or controlled merge into existing target. | Destination options are distinct and audited. |
| **RST-012** | P0 | Expose managed PITR only if source provider and plan offer a supported integration. | UI distinguishes managed PITR from logical backups and shows supported time range. |


## 8. Interfaces, reporting and accessibility

| ID | Priority | Requirement | Acceptance criterion |
|---|---|---|---|
| **UI-001** | P1 | Provide signed native SwiftUI macOS application with dashboard, project setup, jobs, restores and settings. | All essential workflows usable without Terminal. |
| **UI-002** | P1 | Provide responsive secure web dashboard for desktop, tablet and phone. | Core monitoring and authorised restore workflow pass viewport tests. |
| **UI-003** | P1 | Display textual and graphical backup history, success/failure, durations, sizes and storage trends. | Figures reconcile with underlying job records. |
| **UI-004** | P0 | Show per-component recovery readiness: recoverable, manual steps, at risk or unknown. | Status derives from evidence and does not rely on arbitrary percentages. |
| **UI-005** | P1 | Provide searchable/filterable archive catalogue and table/object inventory. | User can find backup by project, date, type and object. |
| **UI-006** | P1 | Provide notifications for failure, overdue backup, low space, replication failure and failed restore drill. | Test event generates notification with actionable context. |
| **UI-007** | P1 | Support accessible controls, keyboard navigation, VoiceOver and WCAG 2.2 AA web target. | Automated and manual accessibility review passes defined checklist. |
| **UI-008** | P1 | Provide exportable audit, backup and recovery reports in CSV/JSON/PDF. | Exports match filtered data and respect role permissions. |


## 9. Security and governance

| ID | Priority | Requirement | Acceptance criterion |
|---|---|---|---|
| **SEC-001** | P0 | Support multi-user identity, MFA/passkeys or OIDC, secure sessions and account recovery. | Unauthenticated and expired sessions are denied. |
| **SEC-002** | P0 | Enforce project-scoped RBAC for Viewer, Operator, Administrator and Recovery Approver. | Permission matrix is enforced server-side for every endpoint. |
| **SEC-003** | P0 | Require step-up authentication and optional dual approval for destructive production restores and key changes. | Requester cannot self-approve when dual control enabled. |
| **SEC-004** | P0 | Protect API using TLS, CSRF defences, rate limits, secure cookies and strict origin controls. | Security tests show no bypass via browser or direct API. |
| **SEC-005** | P0 | Restrict remote access to private network by default; never expose database or admin service ports publicly by default. | Network scan confirms only intended interfaces reachable. |
| **SEC-006** | P0 | Use least-privilege source/cloud credentials and separate backup-write from deletion privileges where supported. | Credential audit confirms permissions are scoped. |
| **SEC-007** | P0 | Record tamper-evident audit events for sign-in, settings, backup, restore, deletion, export and key actions. | Audit entries contain actor, project, time, action and result without secrets. |
| **SEC-008** | P0 | Support sensitive-data classification, configurable log redaction and no backup content in telemetry. | Secret and personal-data leakage tests pass. |
| **SEC-009** | P1 | Support configurable data retention and deletion, privacy notices and access reporting. | Retention and audit policies are independently configurable. |
| **SEC-010** | P0 | Pin dependencies, generate SBOM, scan releases, sign/notarise Mac builds and verify updates. | Release pipeline blocks critical unresolved findings and unsigned binaries. |


## 10. Reliability, operations and non-functional

| ID | Priority | Requirement | Acceptance criterion |
|---|---|---|---|
| **NFR-001** | P0 | Run unattended as launchd-managed service independent of GUI login. | Jobs execute with app closed and after supported reboot sequence. |
| **NFR-002** | P0 | Use crash-safe metadata transactions and reconcile in-progress jobs on restart. | Fault-injection test leaves no falsely completed job. |
| **NFR-003** | P1 | Provide structured logs, metrics, health checks and diagnostic export. | Operator can diagnose a failed backup without secrets. |
| **NFR-004** | P1 | Support at least 20 projects and configurable concurrency without overloading source databases. | Load test meets policy deadlines for documented reference dataset. |
| **NFR-005** | P1 | Use bounded memory and disk working space; detect insufficient free space before dump. | Low-space test fails safely and alerts. |
| **NFR-006** | P0 | Preserve archive readability across supported application upgrades. | N-1 version archive restore succeeds after upgrade. |
| **NFR-007** | P1 | Provide signed installer, migration, rollback and update documentation. | Upgrade and rollback rehearsals preserve catalogue and jobs. |
| **NFR-008** | P0 | Document and measure per-project RPO/RTO, default provisional targets 24h/8h. | Drill records measured values and flags missed objectives. |
| **NFR-009** | P1 | Support UK English and timezone-aware scheduling, including DST. | DST test cases pass without unintended duplicate destructive operations. |
| **NFR-010** | P0 | Support self-hosted deployments with explicit version/capability checks and documented supported topologies. | Representative Docker/self-hosted deployment completes backup and recovery drill. |


## 11. Data model and interfaces

**Core entities:** `projects`, `project_members`, `connections` (secret references only), `capability_assessments`, `component_inventory`, `backup_policies`, `backup_jobs`, `recovery_sets`, `component_archives`, `archive_replicas`, `storage_destinations`, `verification_runs`, `restore_jobs`, `restore_steps`, `approval_requests`, `audit_events`, `notifications`, `application_dependencies`, `schema_migrations`.

**Key invariants:** recovery set completeness is computed from required components and validation evidence; every archive is immutable once published; every replica has independent integrity status; secrets are never embedded in catalogue; project IDs scope all access; restore operations have immutable plans and approvals.

**Representative API (all under `/api/v1`):**
- `GET/POST /projects`, `GET /projects/{id}/capabilities`, `POST /connections/test`
- `GET/POST /backup-policies`, `POST /backup-jobs`, `GET /backup-jobs/{id}`
- `GET /recovery-sets`, `GET /recovery-sets/{id}/manifest`
- `POST /restores/preview`, `POST /restores`, `POST /restores/{id}/approve`, `GET /restores/{id}`
- `GET /audit-events`, `GET /metrics`, `GET /events` (SSE)
- `POST /storage-destinations/{id}/test`, `POST /verification-runs`

All mutating APIs require authenticated project-scoped authorisation, CSRF defence for cookie-based clients, audit logging and idempotency keys for job creation. Destructive endpoints use explicit approval tokens tied to a plan digest and expiry. API contracts will be OpenAPI 3.1, with generated Swift/TypeScript clients.

### 11.1 Recovery-set manifest (conceptual)

```json
{
  "format_version": "1.0",
  "recovery_set_id": "uuid",
  "project_id": "uuid",
  "source_type": "supabase_cloud",
  "capture_started_at": "2026-11-03T02:00:00Z",
  "capture_completed_at": "2026-11-03T02:08:00Z",
  "components": [
    {"type":"postgresql", "status":"verified", "archive_id":"sha256:..."},
    {"type":"storage", "status":"verified", "archive_id":"sha256:..."},
    {"type":"auth_configuration", "status":"manual_steps_required"}
  ],
  "known_gaps": ["external_oauth_secret_requires_reentry"],
  "restore_test_status": "not_tested",
  "manifest_signature": "detached-signature-reference"
}
```

Actual manifest will have a JSON Schema, cryptographic signing specification, object counts, byte counts, version metadata, encryption envelope references, hashes, dependencies and independent per-replica verification results. `status=verified` applies to integrity, not successful recovery.

## 12. Verification and test strategy

| Test ID | Scope | Acceptance evidence |
|---|---|---|
| T-01 | Connection matrix | Cloud and self-hosted versions, direct/pooler/TLS/permission tests |
| T-02 | PostgreSQL recovery | Full, schema, data, table, roles, RLS, extensions and constraints compared |
| T-03 | Storage recovery | Bucket settings and file bytes/hash reconciled; pagination tested |
| T-04 | Auth recovery | User records, identities and defined sign-in journeys verified; unsupported secrets flagged |
| T-05 | Edge Functions | Source revision mapped to deployment; clean redeploy and invocation test |
| T-06 | Project settings | Exportable settings reapplied; manual checklist executed |
| T-07 | End-to-end disaster recovery | Rebuild clean Cloud project and representative self-hosted deployment |
| T-08 | Partial recovery | Selected row/table restore with conflict, FK and rollback scenarios |
| T-09 | Host loss | Recovery CLI restores using off-site archive and escrowed keys |
| T-10 | Archive tampering | Altered ciphertext/manifest rejected; audit event generated |
| T-11 | Storage providers | Upload/download/retry/expiry/immutability and partial failure across all five destinations |
| T-12 | Scheduling/faults | DST, reboot, interrupted upload, low disk, network outage, duplicate trigger |
| T-13 | Access control | Role/project isolation, MFA, step-up approval, session and CSRF tests |
| T-14 | Usability/accessibility | Mac keyboard/VoiceOver, mobile dashboard, WCAG 2.2 AA checks |
| T-15 | Capacity/performance | 20 projects, configurable parallelism, representative large dataset |
| T-16 | Release/upgrade | Signed/notarised install, migrations, rollback, archive backwards compatibility |

**Production exit gate:** all P0 tests pass; no unresolved critical security defects; all six component categories have assessed and tested recovery paths for both environment types; documented unsupported capabilities are visible and accepted per project; at least one independent host-loss drill succeeds; a complete recovery drill produces measured RPO/RTO; operators approve the runbook. No claim of complete automation where manual steps remain.

## 13. Implementation plan and milestones

| Stage | Indicative effort | Work products | Exit gate |
|---|---|---|---|
| M0 Discovery | 2-3 weeks | Supabase Cloud/self-hosted capability matrix, API permissions, source/version compatibility, threat model | Evidence-backed scope and known gaps |
| M1 Platform foundation | 2-3 weeks | Go daemon, launchd installer, SQLite migrations, OpenAPI, SwiftUI shell, React shell, RBAC | Two interfaces connect securely to service |
| M2 Core capture | 3-5 weeks | PostgreSQL/Storage/Auth adapters, recovery manifests, scheduling, checksums | Multi-component recovery sets verified |
| M3 Configuration/functions | 3-4 weeks | Function source adapter, project settings, secrets references, dependency graph | Clean function deployment and configuration reconstruction |
| M4 Cloud repositories | 3-5 weeks | Local, S3, Azure, Google Drive, OneDrive, encryption and key escrow | All destinations tested; fresh-host decrypt succeeds |
| M5 Guided recovery | 4-6 weeks | Full project, table/row, Storage, Auth, function recovery wizard; approvals | End-to-end Cloud and self-hosted restore drills |
| M6 Hardening/release | 3-4 weeks | Monitoring, accessibility, load/security tests, notarisation, docs, upgrade path | All P0 exit gates passed |

**Indicative total:** 20-30 engineering weeks of sequential effort, subject to Supabase API limitations, integration approvals, test environments and team capacity. Parallel work may reduce calendar time. These are planning estimates, not commitments. Do not defer mandatory protection categories from the production release merely to meet a date.

### 13.1 Repository structure

```text
supabase-backup-manager/
  apps/macos/                  # SwiftUI
  apps/web/                    # React / TypeScript
  services/backupd/           # Go API, scheduler, workers, recovery
  packages/api-contract/      # OpenAPI / JSON schemas
  tools/recovery-cli/          # Portable independent recovery
  database/migrations/        # SQLite schema
  adapters/                   # postgres, storage, auth, functions, settings, secrets
  integrations/               # s3, azure, gdrive, onedrive, local
  tests/                      # unit, integration, security, DR drills
  installers/macos/           # launchd, packaging, notarisation
  docs/                       # architecture, operations, runbooks
  .github/workflows/          # CI, SBOM, signed release
```

## 14. Risks, constraints and mitigations

| Risk | Impact | Mitigation |
|---|---|---|
| Managed Supabase secrets or settings cannot be exported | Full automation impossible | Capability assessment, external secret escrow, manual recovery runbook |
| Edge Function deployed source not available from API | Incomplete function recovery | Require linked Git/source archive; block green readiness status |
| Auth restore is version-sensitive | Sign-in failure | Version checks, isolated rehearsals, provider-specific validation |
| Database dump omits Storage object bytes | Incomplete data recovery | Independent Storage object capture and checksum reconciliation |
| Cloud/self-hosted APIs differ | Unsupported operations | Versioned adapters and environment-specific capability matrices |
| Components are not atomically captured | Cross-component inconsistency | Capture window, dependency ordering, reconciliation and recovery tests |
| Credentials/keys lost with Mac | Backups unreadable | Independent recovery-key escrow and fresh-machine drills |
| Cloud storage quotas/rate limits | Missing off-site replica | Backoff, quota monitoring, multipart/resumable upload, per-replica status |
| Remote admin interface compromised | Destructive restore/data exposure | Private network, RBAC, MFA, dual approval, audit and least privilege |
| Restore disrupts live users | Data loss/downtime | Isolated default, impact preview, safety backup, rollback plan |

## 15. Decisions and open technical questions

The following are **implementation discovery items**, not blockers to writing requirements:
1. Exact minimum macOS version and supported Apple Silicon generations.
2. Supported Supabase Cloud plan tiers, API scopes and management endpoints at implementation time.
3. Supported self-hosted topologies and version matrix (Docker, Kubernetes, custom).
4. Auth record portability, password hash compatibility, signing key rotation and external provider cutover.
5. Edge Function source-of-truth policy where no Git repository is linked.
6. Master-key escrow method (offline recovery key, cloud KMS, or both).
7. Whether two-person approval is mandatory for all production restores or only destructive ones.
8. Target backup sizes, throughput, storage budgets and measured RPO/RTO by project.
9. Required privacy/data residency constraints for cloud storage locations.
10. Whether private-network access is sufficient or a future internet-facing identity-aware gateway is required.

## 16. Requirement traceability and change control

- Requirement IDs remain stable; revisions change wording/acceptance criteria without recycling IDs.
- Each implementation ticket references one or more IDs; each P0 ID maps to automated or documented manual test evidence.
- A release report lists implemented, tested, blocked, unsupported and waived requirements.
- Waivers require named owner, reason, risk, expiry and mitigation; P0 safety/recovery gaps cannot be silently waived.
- Changes to recovery-set format, encryption or restore semantics require compatibility testing and documented migration.

## 17. References to validate during engineering discovery

- Supabase documentation: https://supabase.com/docs/guides/platform/backups
- Supabase CLI: https://supabase.com/docs/reference/cli/introduction
- Supabase Storage: https://supabase.com/docs/guides/storage
- Supabase Auth: https://supabase.com/docs/guides/auth
- Supabase Edge Functions: https://supabase.com/docs/guides/functions
- Supabase self-hosting: https://supabase.com/docs/guides/self-hosting
- PostgreSQL pg_dump: https://www.postgresql.org/docs/current/app-pgdump.html
- PostgreSQL pg_restore: https://www.postgresql.org/docs/current/app-pgrestore.html

**Note:** Product/API support, permissions, pricing and limitations must be revalidated against current documentation and tested in both deployment models before implementation is finalised. This document is a complete requirements baseline, not evidence that every proposed export or restore operation is currently supported by Supabase.

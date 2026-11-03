# Product requirements baseline

## Product decisions
- REQ-PLAT-001 [MUST] Support Apple Silicon macOS and an independently running background service.
- REQ-UI-001 [MUST] Provide a native macOS application and secure responsive web dashboard sharing a versioned API.
- REQ-USER-001 [MUST] Support multiple users, least-privilege roles and audit logs.
- REQ-ENV-001 [MUST] Support Supabase Cloud and self-hosted Supabase, with capability discovery and explicit limitations.
- REQ-PROJ-001 [MUST] Support multiple independently configured Supabase projects.
- REQ-BKP-001 [MUST] Capture PostgreSQL schema, data and permitted roles/grants.
- REQ-BKP-002 [MUST] Capture Storage bucket definitions, object bytes and recoverable metadata.
- REQ-BKP-003 [MUST] Capture recoverable Auth data and configuration, identifying inaccessible managed secrets.
- REQ-BKP-004 [MUST] Preserve Edge Function source from an authoritative source repository and deployment configuration.
- REQ-BKP-005 [MUST] Capture exportable project settings, dependencies and configuration drift.
- REQ-BKP-006 [MUST] Maintain secret references and separately secured recoverable secret material; never put plaintext secrets in manifests.
- REQ-BKP-007 [MUST] Group component archives in a signed or authenticated recovery-set manifest with timestamps, checksums and coverage status.
- REQ-BKP-008 [MUST] Support full, schema-only, data-only and selected-table backups; label incremental and PITR as unavailable unless supported infrastructure is configured.
- REQ-SCH-001 [MUST] Support persistent schedules, missed-job detection and configurable retention.
- REQ-STORE-001 [MUST] Support local, AWS S3, Azure Blob, Google Drive and OneDrive repositories.
- REQ-SEC-001 [MUST] Encrypt archives before remote upload, protect credentials and support independent key recovery.
- REQ-RST-001 [MUST] Provide guided whole-project, database, table, selected-record and Storage-object recovery where supported.
- REQ-RST-002 [MUST] Require preview, dependency validation, safety backup and explicit approval before destructive production restores.
- REQ-DR-001 [MUST] Demonstrate recovery without the original Mac mini.
- REQ-OBS-001 [MUST] Report backup success, component coverage, integrity checks, tested restore status and explicit gaps separately.

## Acceptance conventions
A MUST requirement is not complete until its automated test or documented manual evidence is linked in a PR. Tests must cover negative paths and permission failures. Managed Supabase features that cannot be exported must produce a documented recovery step, not a false green status.

**Pending:** Import the complete version 1.0.0 SRS and original requirement IDs. This abbreviated baseline is not a substitute for the complete approved specification.

# Testing and release gates

## CI checks (every PR)
1. Markdown links and formatting.
2. Requirement IDs unique and traceable to test cases.
3. OpenAPI validation and backward compatibility checks when contract exists.
4. JSON Schema validation and fixture tests when manifests exist.
5. SQLite schema migration and foreign-key/integrity checks when schema exists.
6. Secret scanning, dependency scanning and licence checks.
7. Unit and integration tests for changed implementation code.

## Milestone gates
- **M0 Discovery:** Document verified export/import capabilities for Cloud and self-hosted projects, including unsupported managed settings.
- **M1 Platform:** Daemon survives UI exit and reboot; local and remote access controls tested.
- **M2 Capture:** Database, Storage and Auth backups verified with failure injection.
- **M3 Configuration:** Edge Function source, settings and secret recovery paths verified.
- **M4 Storage:** Local, S3, Azure, Drive and OneDrive round-trip encrypted archives; key-loss drill.
- **M5 Guided recovery:** Full-project, table and selected-record restore previews; approval and rollback behaviour tested.
- **M6 Release:** Independent machine recovery, security assessment, disaster-recovery rehearsal, documentation and signed macOS package.

## Production release blockers
No release if a required component is silently omitted, a backup is marked recoverable without evidence, secrets are exposed, destructive restores bypass approval, or off-site archives cannot be recovered without the original Mac.

Release evidence must record requirement ID, test ID, environment, execution date, result, artefacts and reviewer.

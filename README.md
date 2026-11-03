# Supabase Backup Manager

Open-source disaster recovery manager for **Supabase Cloud and self-hosted Supabase**, designed for Apple Silicon macOS with a native SwiftUI application, a Go background service and a secure web dashboard.

> **Status: specification / pre-implementation.** No production backup or recovery functionality is available. Do not rely on this repository to protect live data.

## Scope
The system must account separately for PostgreSQL, Storage objects, Auth records and configuration, Edge Functions, project settings and secrets. Export limitations must be reported, never hidden. Guided restoration requires previews, approvals, audit evidence and tested recovery procedures.

## Engineering governance
- [Engineering baseline](docs/README.md)
- [Requirements](docs/requirements/BASELINE.md)
- [Architecture](docs/architecture/DECISIONS.md)
- [Test and release gates](docs/testing/RELEASE_GATES.md)
- [Contribution guidelines](CONTRIBUTING.md)
- [Security policy](SECURITY.md)

All changes should be made through pull requests. Requirements and architectural decisions must be linked to tests. Never commit real credentials, archives, personal data or secrets.

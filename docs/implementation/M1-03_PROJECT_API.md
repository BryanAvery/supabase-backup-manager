# M1-03 Local project API

Draft implementation for issue #21. The application has a versioned project API backed by the local SQLite catalogue.

## Endpoints

- GET /api/v1/health: process health only.
- GET /api/v1/projects: membership-scoped project list for active users.
- GET /api/v1/projects/{projectId}/capabilities: latest status per component, subject to independent membership check. Raw evidence is not returned.
- POST /api/v1/projects: requires a trusted identity with explicit project-provisioning permission. Creates the project, administrator membership and audit entry in one transaction.

## Safety

M1-03 intentionally does not implement sign-in. The executable uses a deny-all identity provider, so every management request returns 401. The only identity provider able to authorise calls is injected within automated tests. This prevents accidental deployment of a header-based login shortcut.

The service remains loopback-only. Browser mutations are disabled until M1-04 delivers sessions, origins, CSRF protection and rate limiting. No real Supabase projects or credentials are used.

## Verification

Automated tests check missing and fabricated identities, restricted roles, active/disabled users, cross-project access, missing IDs, catalogue write transactions, input validation, error responses and evidence redaction.

M1-04 will supply production authentication and complete the security requirements. M1-05/06 add UIs and M1-07 tests launchd on an actual Apple Silicon Mac.

The current API is not a backup product and must not be described as production ready.

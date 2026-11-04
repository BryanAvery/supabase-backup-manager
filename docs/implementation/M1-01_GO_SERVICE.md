# M1-01 - Initial Go service scaffold

**Status:** Implementation slice only; not M1 approval. Issue: [M1-01](https://github.com/BryanAvery/supabase-backup-manager/issues/19).

## What is available

- A dependency-free Go executable at cmd/backup-manager.
- An unauthenticated **process-only** health endpoint at GET /api/v1/health.
- Strict loopback-only TCP binding. The default is 127.0.0.1:8765 over plaintext HTTP for local development **only**; no remotely accessible route exists in this slice.
- Bounded HTTP timeouts, basic response safety headers and a graceful SIGTERM/SIGINT shutdown.
- Go unit tests for health response, denied management routes and rejected non-loopback bind addresses.

## Developer usage

Run these from repository root:

1. go test ./...
2. go vet ./...
3. go run ./cmd/backup-manager

From a second terminal: curl --fail --silent http://127.0.0.1:8765/api/v1/health

The endpoint returns {"status":"ok","version":"0.1.0-dev"}. It means only that the process is serving HTTP. **It does not mean the catalogue, authentication, backups, restore paths or Supabase sources are available.** The health server currently has no connection to Supabase or SQLite and must not be marketed or deployed as a backup service.

## Deferred work

- [M1-02](https://github.com/BryanAvery/supabase-backup-manager/issues/20) implements SQLite catalogue/migrations, integrity checks and rollback.
- [M1-03](https://github.com/BryanAvery/supabase-backup-manager/issues/21) adds the authenticated, project-scoped API.
- [M1-04](https://github.com/BryanAvery/supabase-backup-manager/issues/22) adds identity, sessions, CSRF, RBAC and request throttling.
- [M1-07](https://github.com/BryanAvery/supabase-backup-manager/issues/25) installs the daemon via launchd and validates post-reboot operation on Apple Silicon macOS.
- TLS/private-overlay remote access is **not** implemented. No bearer credentials should be sent to this process.

## Verification evidence

CI is expected to build and run go vet and go test -race ./.... Manual Mac service restart/reboot and Keychain access tests are still required. Do not mark the full M1-A01/A02/A08/A11 gates as passed without their complete evidence.

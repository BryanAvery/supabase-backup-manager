# M1-02 - SQLite operational catalogue

**Status:** implementation candidate; not M1 release acceptance. Issue [#20](https://github.com/BryanAvery/supabase-backup-manager/issues/20).

The catalogue lives in a **local SQLite file**, not the Supabase Cloud PostgreSQL database. It stores metadata only; never put passwords, tokens or backup archives into it. Project-scoped RBAC is **not** implemented until M1-03/M1-04. Database code is internal-only and not exposed through an API.

## Design

- The initial embedded migration matches the canonical `docs/spec/v1.0.0/data/schema.sql` after stripping its connection-scoped PRAGMA statements; a parity test enforces this. Do not edit the original specification to fix a migration.
- Go `database/sql` with pinned `github.com/mattn/go-sqlite3`; **CGO** is required. Apple Silicon production builds require Xcode command line tools and native macOS CI/release verification.
- One SQLite database connection per Store for consistent `foreign_keys=ON`, WAL and busy timeout. A full integrity check is performed at opening and after migration.
- Migration files are immutable and embedded; versions and SHA256 checksums are registered transactionally, with unknown versions or checksum mismatches rejected. A failed migration rolls back both DDL and registration.
- Only private absolute paths and private regular database files are accepted. A new file has mode 0600. The eventual installer must provide a dedicated least-privileged private directory; this API does not make an existing path safe if its parent is exposed.
- `Backup(ctx, /absolute/unused/path.sqlite)` uses SQLite `VACUUM INTO` to create an online, consistent, private snapshot. A snapshot is verified and the method refuses overwrite or source=destination. These are **catalogue** snapshots, not Supabase backups.

## Rollback drill for a later migration

1. Stop management service and all catalogue writers (future M1-07).
2. Before schema upgrade, create a snapshot with `Backup` into a new private location, independently verify it and retain its recorded version/checksum.
3. Apply the upgrade. If it fails, the migration transaction rolls back; verify catalogue integrity and migrate status.
4. If the new app must be rolled back after a committed schema upgrade, **stop the new service** and restore the pre-upgrade snapshot as a complete SQLite database copy, rather than inventing a reversing SQL script. Preserve the failed/current database for investigation.
5. Restart the matching old service version, open the restored file and verify original rows and `schema_migrations` state.

**Important:** Database snapshot restoration after a real, committed schema upgrade is an operational procedure, not a production-safe automated rollback in this PR. M1-07 must prove it on the target Apple Silicon host. Existing catalogues with new migration versions are intentionally rejected when opening older application binaries.

## Tests

Run `go test ./...`, `go vet ./...`, and `go test -race ./...`. Tests cover baseline-schema parity, fresh/repeated migration, foreign-key enforcement, atomic rollback on failed DDL/data statement, changed and unknown migration rejection, snapshot recovery and file-path safety. The repository GitHub Actions workflow is the authoritative Go dependency and build verification.

No Supabase projects, credentials, hosted PostgreSQL schemas, or live backups are involved.

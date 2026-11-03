# Architecture decisions and constraints

## ADR-0001 - Host and interfaces
**Status:** Proposed baseline.
Run a Go backup daemon under macOS launchd independently of the SwiftUI desktop app. Serve a React/TypeScript dashboard through a restricted authenticated API. Use SQLite for operational metadata and macOS Keychain for host secrets. Support a portable recovery utility.

## ADR-0002 - Component-based recovery
**Status:** Proposed baseline.
Separate adapters for PostgreSQL, Storage, Auth, Edge Functions, project settings and secrets. Every recovery set has an authenticated manifest referencing separately encrypted archives, with a consistency window and explicit capability gaps.

## ADR-0003 - Remote access
**Status:** Proposed baseline.
Use private network access (for example Tailscale) by default. Do not expose the management API directly to the public internet. Enforce MFA/passkeys, RBAC, rate limits and re-authentication for destructive actions.

## ADR-0004 - Guided restore
**Status:** Proposed baseline.
Restore into an isolated environment by default. For in-place production changes require dependency checks, preview, fresh safety backup, authorisation and immutable audit evidence. Auth/session compatibility and Edge Function deployment must be validated before cutover.

## ADR-0005 - Backup storage
**Status:** Proposed baseline.
Use encrypted portable archives, multiple replicas, checksums, independently recoverable encryption keys and optional immutable object storage. Retention must not remove the last verified recovery point.

## Decision records
Future deviations require a separate `ADR-NNNN.md` documenting context, options, decision, consequences, security implications and test changes.

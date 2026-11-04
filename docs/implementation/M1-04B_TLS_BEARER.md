# M1-04B (part 1): Trusted TLS transport and bearer API authentication

**Status:** Implementation slice for [M1-04B issue #32](https://github.com/BryanAvery/supabase-backup-manager/issues/32), not the completed login or M1 release gate.

This change connects the existing RFC 9068 access-token verifier to the running Go API **only** with explicit, complete identity settings and TLS 1.3 enabled. The default remains the previous health-only or deny-all development mode. No interactive login, new account, MFA provider or browser session is created automatically.

## Security controls

- The daemon binds exclusively to a numeric loopback IP. No public interface, remote listener or proxy trust is added.
- `-enable-bearer-auth` requires an existing private SQLite catalogue, a TLS server certificate and private key, and *all* verifier fields: exact HTTPS issuer, API audience, client ID, required MFA ACR, JWT key ID and pinned RSA PUBLIC KEY.
- All authenticated requests go over actual TLS, **not** the `X-Forwarded-Proto` header. The server uses TLS 1.3 minimum; the verifier separately refuses requests with no TLS.
- The server certificate must have the exact numeric loopback listener address in its IP subject alternative name, valid server-authentication usage and current validity dates. The private key must be a regular file with no group/other permissions; symlinks are rejected.
- Project membership and administrator rights are rechecked in the catalogue; revoked token fingerprints, disabled users, expired or incorrect-audience JWTs and forged role headers are denied.
- An intentionally missing `-enable-bearer-auth` keeps management routes at HTTP 401 even when a catalogue is configured.
- Browser-origin write requests remain blocked until the complete session/CSRF implementation; no cookies are issued. The public health endpoint is process status only, not backup or authentication readiness.

## Demonstrating the secure boundary

Create a **trusted local-only** server certificate (the certificate must include IP SAN `127.0.0.1`) and keep its private key mode 0600. Obtain an approved IdP's **public** RSA signing key via a trusted provisioning channel. Never place private signing keys, bearer tokens or client secrets in source control, command-line arguments or documentation.

An example command shape, with placeholder public configuration only:

```bash
go run ./cmd/backup-manager \
  -listen 127.0.0.1:8765 \
  -catalogue /private/absolute/path/catalogue.sqlite \
  -tls-cert /private/absolute/path/localhost.crt \
  -tls-key /private/absolute/path/localhost.key \
  -enable-bearer-auth \
  -auth-issuer https://issuer.example.invalid/ \
  -auth-audience api://backup-manager \
  -auth-client-id approved-native-client \
  -auth-required-acr urn:approved:mfa \
  -auth-key-id approved-key-id \
  -auth-public-key /private/absolute/path/issuer-public.pem
```

**The example above is deliberately incomplete for an actual identity provider**: the host, audience, client ID, ACR and key ID must be approved, supported and provisioned before use. In particular, not every IdP issues an RFC 9068 `at+jwt` access token with a signed `acr`. Do not bypass the required claims to make a provider work.

Catalogue accounts are **offline-provisioned** with `external_subject` constructed using the Go function `identity.ExternalSubjectKey(issuer, subject)`. Instance administrators are separately granted in `instance_administrators`; no remote endpoint permits self-provisioning or elevation. Privilege removal in SQLite takes effect on the next privileged API request.

For health-only TLS diagnostics, use a locally trusted certificate and verify it (do not use `curl -k` when transmitting credentials). The API does not create or restore a Supabase backup.

## Automated evidence and limitations

- Negative config tests prove missing TLS/issuer/MFA policy, public listener, expired/mismatched certificate, group-readable/symlinked private key and incorrect auth configuration are rejected.
- A real in-process TLS 1.3 server accepts a signed synthetic API access token only after the user is provisioned in SQLite. Tests then verify per-project isolation, missing administrator privilege, cross-origin write denial, revocation and disabled-account rejection.
- All tests use generated in-memory synthetic signing keys and disposable SQLite fixtures; no Supabase project, credentials or real backup data are touched.
- Full M1-04B still needs review of proposed ADR-0002, real IdP selection, authorisation-code plus PKCE login, secure cookies/native session management, CSRF and origin enforcement, complete denial auditing, service key rotation and a real Mac/Keychain/launchd test.

**No claim of M1-04, T-13 or production acceptance is made by this PR.**

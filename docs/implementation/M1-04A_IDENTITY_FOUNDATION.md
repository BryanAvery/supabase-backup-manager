# M1-04A - Identity verification and RBAC foundations

**Scope:** This is the first M1-04 implementation slice, not the completed login or security milestone.

## Implemented

- Strict RFC 9068 JWT *access-token* verifier using a pinned RSA public key. Requires RS256, issuer, API audience, client ID, signing key ID, short lifetime, signed MFA assurance (`acr`) and an active pre-provisioned local identity. ID tokens and token-supplied role claims do not grant access.
- SQLite migration `0002_access_control.sql` adds explicit instance administrator grants and token revocation fingerprints. No administrators are created automatically. The API rechecks local administrator authority for every project creation.
- Local bearer-token revocation route `POST /api/v1/sessions/revoke`, plus rejection of locally revoked tokens by the JWT verifier.
- Local per-client rate limiter and continued rejection of browser-Origin mutations until CSRF is implemented.
- Negative tests for expired, foreign-key-signed, incorrect-issuer, incorrect-audience, unprovisioned, non-MFA and revoked tokens; role grant removal and basic request throttling.
- A proposed architecture decision, ADR-0002, to review identity provider and credential transport before enabling live authentication.

## Critical limits

The executable **still uses the deny-all authenticator**. The JWT verifier is deliberately not wired to the running HTTP listener: accepting bearer credentials over plaintext HTTP would expose them. There is no login UI, PKCE callback, TLS service, browser session, IdP configuration, or complete CSRF scheme yet. M1-04B must deliver these before real accounts are enabled.

This code does not connect to or mutate any hosted Supabase project. It is not sufficient to mark M1-04 or T-13 complete.

## Evidence

GitHub Actions checks for Go build, vet, race tests and the existing specification suite. The new RSA signing keys in tests are generated in memory and are not persisted or committed.

## Follow-up

- Choose and configure an OIDC provider with verified MFA claims and API-scoped audience.
- Add TLS/private transport, interactive authorisation-code + PKCE, secure browser/native sessions, rigorous session expiry and logout.
- Add full denial audit logging and stronger origin/CSRF handling before any browser write.

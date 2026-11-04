# M1-04B, slice 2: Browser OIDC PKCE sign-in and short-lived sessions

**Status:** Feature implementation and synthetic validation, not production identity approval or completed M1-04.

This slice extends the merged TLS 1.3 and verified bearer-token implementation in PR #33. Interactive browser sign-in remains an **explicit opt-in** that requires all previously configured TLS/JWT settings, plus a pinned HTTPS OIDC authorisation endpoint, token endpoint, callback URI, API scope and separate ID-token signing key.

## Browser authentication flow

1. The browser visits `GET /web/login` on the local TLS listener. A 302 redirect starts a **single-provider** OIDC authorisation-code flow with a fresh cryptographically random state, browser-bound `__Host-sbm-login` cookie, nonce and PKCE S256 code challenge. No implicit grant, password grant, refresh token scope or open redirect is permitted.
2. The provider redirects to the **exact** `https://127.0.0.1:<port>/web/auth/callback`. The callback requires the pre-registered `state`, a matching browser cookie and the exact issuer response `iss`. State is consumed once and expires after five minutes.
3. The server exchanges the code and PKCE verifier with the pinned HTTPS token endpoint over a bounded request. Redirecting the exchange to another URL is not permitted. The authorisation code necessarily appears briefly in the callback URL under the OAuth code flow; the service exchanges it server-side and redirects to a fixed clean path with a no-referrer policy. Access tokens and ID tokens are never sent to JavaScript or placed in browser URLs/cookies.
4. Both returned credentials are checked independently. The ID token must have an approved RSA signature, key ID, issuer, client audience, nonce, expiry and required signed MFA `acr`. The **separate** API access token must meet the existing RFC 9068 verifier policy, including exact API audience, client ID, short expiry and local revocation. The two tokens must resolve to the **same active pre-provisioned catalogue user**.
5. A random opaque `__Host-sbm-session` cookie is issued with `Secure`, `HttpOnly`, `SameSite=Lax` and root path attributes. Only its SHA-256 fingerprint is used as an in-memory lookup key; no OAuth tokens are stored in browser cookies or session records. Session expiry is capped at 15 minutes and the shortest token lifetime. Memory sessions **fail closed on daemon restart**.
6. `GET /web/session` returns session expiry and a CSRF value to same-origin JavaScript. `GET /web/projects` returns only projects still visible to the active local user. All browser project mutations remain unavailable. `POST /web/logout` requires exact same-origin `Origin` plus the CSRF value, invalidates the opaque session and writes local access-token revocation.

All browser routes are TLS 1.3-only, require the exact configured Host and are subject to the existing request limiter. Responses use no-store, no-referrer, clickjacking protection and restrictive content security policy. The API bearer routes remain separate and require the existing verified access token. Forwarded headers or a user-supplied ID never grant identity.

## Configuration, without credentials

In addition to the M1-04B(1) flags already documented, configuring browser login requires:

- `-enable-browser-login`
- `-oidc-authorize-url https://<approved-issuer-host>/authorize`
- `-oidc-token-url https://<approved-issuer-host>/token`
- `-oidc-redirect-uri https://127.0.0.1:8765/web/auth/callback`
- `-oidc-api-scope <approved-api-scope>`
- `-oidc-id-key-id <pinned-id-token-key-id>`
- `-oidc-id-public-key /absolute/path/approved-id-public.pem`

The identity provider **must not share the loopback application's hostname**, even on a different port. Browser cookies are scoped to hostnames, not ports. The test harness uses separate IPv4 and IPv6 loopback hosts to prove this isolation. The fixed endpoints and S256 flow follow [RFC 9700](https://www.rfc-editor.org/rfc/rfc9700).\n\nThe `-auth-client-id` must refer to a registered **public PKCE client**; no client secret is accepted by this slice. The identity provider must explicitly support all required signed MFA assertions, the pinned access-token/ID-token profiles, PKCE S256, the issuer response `iss` and the fixed localhost redirect. **No provider has been selected or configured on behalf of the user.** An incompatible provider must lead to a reviewed protocol change, never relaxed signature, audience or MFA checks.

## Verification

Automated tests use an in-process synthetic HTTPS OIDC token issuer with generated RSA keys. They test PKCE S256, state/cookie binding, replay prevention, issuer/nonce/identity mismatch, controlled redirect, HTTPS-only enforcement, opaque session cookie attributes, project visibility, logout with CSRF/origin, short expiry and disabled identity. Prior token-verifier, project-isolation and SQLite tests continue to run.

## Deferred for the full M1-04 gate

- Approve ADR-0002 and a genuine OIDC provider, registration, MFA claim profile and IdP logout behaviour.
- Assess provider discovery, key rotation, HTTP client certificate pinning/rotation and supported native PKCE application callbacks.
- Persist an approved session/denial audit trail and implement any required separate server-side browser project-write CSRF model. No browser project write is enabled here.
- Confirm reverse-proxy/private networking policy (none is trusted in this slice), macOS Keychain, real Apple Silicon launchd and multi-user session acceptance.
- Review the feature for deployment security before anyone uses real credentials.

**No live Supabase projects, user accounts, credentials or backup data are used by this slice.** Passing CI will not establish M1 release readiness or T-13 sign-off.

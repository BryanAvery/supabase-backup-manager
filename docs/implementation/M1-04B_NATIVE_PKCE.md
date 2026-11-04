# M1-04B native macOS PKCE and Keychain foundation

**Scope:** Buildable Swift macOS authentication library for the future SwiftUI client, plus an authenticated Go identity-binding route. Not a finished Mac app, and not approval to enable real accounts or production backups.

## Architecture

The Swift package is at `native/AuthFoundation`, with no third-party dependencies. It targets macOS 13 or later and uses Apple's AuthenticationServices, Security and CryptoKit frameworks.

The native app, once created in M1-05, must register its **developer-controlled reverse-domain** callback URI scheme in `CFBundleURLTypes`, provide a real `NSWindow` to `MacOSBrowserAuthorizer`, and supply explicit OIDC public-client configuration. The app uses `ASWebAuthenticationSession` rather than an embedded web view. The chosen provider must support custom-scheme redirect URIs, code+PKCE S256, a signed MFA `acr`, OIDC ID tokens and API-scoped RFC 9068 access tokens. There is no client secret or refresh-token flow.

### Sign-in protocol

1. Generate independent 256-bit random state, nonce and S256 verifier. Use an external system browser, a fixed pre-registered reverse-domain callback URI and a five-minute one-use state.
2. Validate callback URI, one copy of each required query parameter, state and issuer response `iss`; consume state even on failure.
3. Exchange the code at the configured HTTPS token endpoint, using the saved PKCE verifier, without following redirects. Reject invalid content type, response size and token lifetime.
4. Verify the OIDC ID token using a **pinned PKCS#1 DER RSA public key**, RS256 signature, expected key ID, issuer, client audience, matching nonce and signed MFA `acr`. Ignore untrusted JWT key-discovery headers.
5. Send the independently received **API access token** to `GET /api/v1/me` on the configured TLS loopback service. The Go server verifies its own JWT signature, issuer, audience, client, MFA, expiry, active catalogue identity and local revocation, then returns `{user_id, subject}`. Compare its verified `subject` with the signed ID-token subject before accepting the login. Never use a client-supplied user-id header to authorise access.
6. Only then write the API access token with a capped expiry to the device-local macOS Data Protection Keychain. The item uses `kSecAttrAccessibleWhenUnlockedThisDeviceOnly` and disables synchronisation; no token, verifier, code or ID token is written to an app preferences file or browser cookie.
7. Each permitted project read uses the bearer token over TLS; the server re-evaluates permissions. Logout asks the API to revoke the token and clears the local Keychain entry even if server-side revocation cannot be confirmed. The caller receives `false` when remote revocation was not confirmed.

### Restrictions

- API origin must be an HTTPS numeric loopback address with a fixed port and `/api/v1` path. The TLS certificate must be trusted by macOS. There is **no** certificate-verification bypass or `X-Forwarded-Proto` identity shortcut.
- OIDC issuer and authorisation/token endpoints must use HTTPS and a pinned host/port; the token transport forbids redirects.
- The callback uses a reverse-domain custom URI scheme, which requires genuine ownership and registration in an app bundle. A scheme is a public identifier, not an authentication secret. PKCE protects intercepted codes. Private-use scheme collisions and native app signing must be validated during M1-05.
- Keychain access requires an appropriate macOS app identity, entitlement and signed application context. CI uses a mock credential store; **real Keychain unlock/lock, crash, reinstall and multi-user tests remain pending** on Apple Silicon.
- The Swift package is a library, not a signed `.app` or an actual interactive production login. Real provider registration, native UI, callback registration, token-refresh strategy, key rotation and account onboarding are not delivered here.
- No production Supabase data, recovery action, IdP credentials or real user accounts were used.

## Automated checks

```bash
swift build --package-path native/AuthFoundation
swift test --package-path native/AuthFoundation --parallel
go test -race -count=1 ./...
```

GitHub Actions `m1-native-auth.yml` runs Swift on macOS, capturing the toolchain and architecture. Go CI tests the new verified identity endpoint with synthetic signed access tokens.

Swift tests cover PKCE S256, state replay and replacement, five-minute expiry, callback scheme/issuer rejection, pinned ID-token signing, MFA/nonce/subject checks, API identity cross-check, mocked credential handling, read-only project access and logout success/failure.

## Release gates still outstanding

- M1-04: approval of ADR-0002, actual IdP registration/MFA claim contract, browser/native auth audit denials, key rotation, real user provisioning and full session security review.
- M1-05: signed native SwiftUI app, URL handler registration and Keychain entitlements.
- M1-07: real Apple Silicon service and native-app restart/reboot tests.

**Passing macOS CI is not a substitute for an Apple Silicon device acceptance test or T-13 sign-off.**

References: [RFC 8252, OAuth for Native Apps](https://www.rfc-editor.org/rfc/rfc8252.html), [Apple ASWebAuthenticationSession](https://developer.apple.com/documentation/authenticationservices/aswebauthenticationsession), and [Apple Data Protection Keychain](https://developer.apple.com/documentation/security/ksecusedataprotectionkeychain).

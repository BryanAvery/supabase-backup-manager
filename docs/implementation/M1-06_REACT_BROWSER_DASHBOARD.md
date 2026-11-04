# M1-06: Read-only React browser dashboard

**Status:** Development slice. M1-06 and M1-A09 acceptance stay open until production-style local TLS and real browser/IdP tests are recorded.

This implementation adds a responsive React/TypeScript dashboard that displays **only the project records authorised by the same local Go catalogue service used by the native app**. No backup, restore, connection, secrets, role-management or project-create controls are enabled.

## Architecture and security boundaries

- The browser UI lives in `web/` and is built with pinned React, TypeScript and Vite. Vite's production base is `/app/`, with no external CDN or third-party assets. JavaScript contains **no bearer token, refresh token or identity-provider secret**.
- The Go daemon only serves a **separately enabled production build** under `https://<numeric-loopback>:<port>/app/`. Use `-web-assets-dir` to point to the absolute Vite `web/dist` directory. Without this flag the new UI is not served. `-enable-browser-login`, private TLS, the approved JWT verifier, pinned OIDC parameters and an existing local catalogue are also required.
- The daemon reads built assets once at startup. The directory must be regular, not symlinked or writable by group/other; only bounded `index.html`, `assets/*.js` and `assets/*.css` are loaded. It rejects symlinks, unexpected extensions, oversized contents and arbitrary request paths. Nothing from the source tree is served to the browser.
- Every `/app/` route enforces actual TLS 1.3 and the exact loopback `Host`, not untrusted forwarding headers. HTML uses a strict same-origin Content Security Policy, frame restrictions and `no-store`/no-referrer headers. No CORS origins or public network listeners are added.
- Browser sign-in still uses `GET /web/login`, the OIDC PKCE state/nonce and server-side `/web/auth/callback`. When UI assets are enabled, a successful callback redirects to `/app/`. The client calls only `GET /web/session`, `GET /web/projects` and `POST /web/logout`, using fixed same-origin paths, secure HttpOnly browser cookies and an in-memory CSRF proof for logout. The server enforces Origin and CSRF, and the UI treats unconfirmed sign-out as a warning, **not success**.
- The client never stores auth or project data in `localStorage`, `sessionStorage`, IndexedDB or a service worker. Project rows are cleared on 401, revocation, expiry, a malformed response or a network failure. No fixture rows are included in the UI.
- Recovery readiness is displayed as a **catalogue assessment**, not a proven successful backup. Disabled future features are labelled as future work, not linked or simulated.

## Build

A clean checkout with Node.js 22 or newer:

```bash
cd web
npm ci
npm run check
npm test
npm run build
```

The build must create `web/dist/index.html` and hashed `web/dist/assets/*.js` / `*.css`. Only serve this production build; the Vite development server is **not** suitable for secure authenticated deployment. The committed `web/package-lock.json` pins and integrity-checks the full dependency tree. CI installs exactly those packages with `npm ci`.

To make the UI available, an operator must first securely configure the Go daemon as documented in `docs/implementation/M1-04B_TLS_BEARER.md` and `docs/implementation/M1-04B_OIDC_BROWSER_SESSIONS.md`, then add:

```bash
-web-assets-dir /absolute/path/to/supabase-backup-manager/web/dist
```

The endpoint is then `https://127.0.0.1:8765/app/` for the sample loopback listener. The app remains locked unless a genuine IdP has been selected, registered, approved and verified to issue exactly the required MFA-protected JWT profiles. The application does **not** create users or enable administrator rights automatically.

## Validation

- `web/tests/api.test.mjs`: strict session/CSRF and project contracts, UUIDs, disabled invalid readiness status, fixed same-origin URL policy, no bearer exposure, bounded JSON content and logout failure.
- `web/tests/controller.test.mjs`: unauthenticated/expired accounts, revoked membership, network outages, late fetch responses, account switching and unconfirmed sign-out.
- `internal/websession/assets_test.go`: real TLS and exact Host enforcement, CSP, asset MIME/size/type restrictions, directory/symlink denial and path traversal.
- `internal/websession/websession_test.go`: synthetic TLS OIDC transaction establishes a server session and redirects to the private dashboard.
- `cmd/backup-manager/config_browser_test.go`: the UI cannot be enabled without complete OIDC, MFA and TLS settings.
- CI `.github/workflows/m1-web.yml`: strict TypeScript checks, Node negative-path tests, production Vite build, asset inspection and build evidence. Existing Go race tests must also pass.

## Known limitations and outstanding gates

1. No identity provider is approved. Real OIDC credentials, provider profile, key rotation, comprehensive denial auditing and IdP logout still need M1-04 review.
2. The UI is available on **local numeric loopback only**. Remote browser access, private overlay networking, reverse proxies and their identity assumptions are **not** implemented or approved.
3. Browser integration should be exercised manually on an Apple Silicon Mac with the native TLS service and approved identity provider, including page refresh, restart, user revocation, expired MFA, CSRF denial and service outage. This is required before M1-A09 acceptance.
4. Server capability and backup/restore APIs are intentionally not exposed through the browser UI.
5. No production Supabase service, secret, dataset or backup was used in the tests.

The M1-06 implementation can be merged when source tests and CI pass, but that is not a declaration of production readiness or T-13/M1 gate closure.

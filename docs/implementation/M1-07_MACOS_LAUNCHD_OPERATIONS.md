# M1-07 - macOS launchd deployment and headless operational runbook

**Status:** Installer staging and headless test implementation. **Not yet approved for live installation, real IdP use, or M1-A02/A08/A10.**

The M1 service must continue running when the SwiftUI app closes, users log out, or the display sleeps. A system-level `LaunchDaemon`, **not** a per-user `LaunchAgent`, supports that operating model. The daemon has no graphical interface and **must not access any user's interactive Keychain**. Native app credentials remain in the individual logged-in user's Keychain. The Go backend reads its separate, operator-provisioned TLS key from a private file. See [Apple Service Management](https://developer.apple.com/documentation/servicemanagement) and [Apple's launchd guidance](https://support.apple.com/guide/terminal/script-management-with-launchd-apdc6c1077b-5d5d-4d35-9c19-60f2397b2369/mac).

**Everything below is for a disposable, explicitly approved Mac test environment. Do not point it at production Supabase projects or enable any real identity provider without M1-04 security approval.**

## 1. Artefacts and safety model

- `deploy/macos/launchd.py`: standard-library-only **non-destructive** plist renderer, plus read-only macOS permissions audit. Neither command performs an install, invokes `launchctl`, creates users, downloads code, enables a listener, or deletes data. Generation refuses to write directly to `/Library/LaunchDaemons` or overwrite a staging file.
- `deploy/macos/examples/locked-local.json`: deliberately **health/deny-all**, no OIDC account, no bearer authentication or browser configuration. It uses numeric loopback and TLS only.
- `deploy/macos/headless_smoke.py`: direct-process test with synthetic one-day RSA TLS, disposable SQLite, clean SIGTERM and recovery after SIGKILL. No actual Supabase or IdP.
- `.github/workflows/m1-daemon.yml`: independent safety-policy tests, real macOS native Go build, `plutil -lint` and headless smoke test. It prints the runner's actual CPU architecture. **CI does not bootstrap a privileged system daemon** and cannot assert GUI logout, reboot or Apple Silicon ownership.
- The Go binary refuses to run as root, even if launchd is misconfigured, and applies an explicit `umask 077` before touching SQLite.

Only a dedicated **non-admin, non-login** local service user (example `_sbmbackup`) can run the daemon. The deployment operator must provision that account separately, with a noninteractive shell such as `/usr/bin/false`. The launchd property list has `UserName`, `RunAtLoad`, `KeepAlive`, restart throttling, restrictive umask and core-dump limits. It has **no** `EnvironmentVariables`, GUI `SessionCreate`, Keychain token, client secret, password, refresh token or access token.

The label `org.example.supabasebackupmanager.daemon` is a **temporary local-development identifier**. A deployment owner must review and replace it with an identifier they control before general distribution. This is separate from the SwiftUI app's example callback scheme.

## 2. File layout, permissions, and prerequisites

The fixed local installation root is:

```text
/Library/Application Support/SupabaseBackupManager/
  bin/backup-manager                  root:wheel        executable, 0555
  config/loopback.crt                 root:wheel        public cert, 0644
  config/access-public.pem            root:wheel        optional public verifier key
  config/id-public.pem                root:wheel        optional public OIDC ID key
  private/loopback.key                _sbmbackup:<primary group>  private key, 0600
  private/catalogue.sqlite            _sbmbackup:<primary group>  SQLite, 0600
  logs/stdout.log                     _sbmbackup:<primary group>  private
  logs/stderr.log                     _sbmbackup:<primary group>  private
  web/dist/                           root:wheel        optional trusted production assets
```

Directory ownership and modes: application root, `bin` and `config` are `root:wheel 0755`; `private` and `logs` are owned by the dedicated service user and mode `0700`. Do not grant write access to those root-controlled directories to app users or the daemon. If the web UI is enabled, `web/dist` must be root-owned, not writable by any other identity, and pass the Go static-asset validator. The daemon **must not run with sudo**.

Only an authorised Mac administrator should provision the service account, system files and LaunchDaemon. Confirm the service user is **not** in the `admin` group and its login shell is noninteractive:

```bash
id _sbmbackup
dscl . -read /Users/_sbmbackup UserShell
id -Gn _sbmbackup
```

Do not silently create the service identity in an installer. Group, UID, login restrictions, FileVault unlock policy and other enterprise controls vary by Mac and need review.

## 3. Build and stage offline (no installation yet)

On a Mac with Xcode Command Line Tools, Go 1.27.x, Python 3.12 and OpenSSL:

```bash
cd /path/to/supabase-backup-manager
go test ./cmd/backup-manager ./internal/catalogue ./internal/server
go build -trimpath -o "$HOME/Downloads/backup-manager-staged" ./cmd/backup-manager

mkdir -p "$HOME/sbm-stage"
cp deploy/macos/examples/locked-local.json "$HOME/sbm-stage/deploy.json"
chmod 600 "$HOME/sbm-stage/deploy.json"

python3 deploy/macos/launchd.py render \
  --config "$HOME/sbm-stage/deploy.json" \
  --output "$HOME/sbm-stage/org.example.supabasebackupmanager.daemon.plist"

plutil -lint "$HOME/sbm-stage/org.example.supabasebackupmanager.daemon.plist"
python3 -m unittest discover -s deploy/macos -p 'test_*.py' -v
python3 deploy/macos/headless_smoke.py --binary "$HOME/Downloads/backup-manager-staged"
```

**No signed app, OIDC account, service process, launchd job or remote connection is installed by these steps.** The smoke script uses a self-generated throwaway certificate only for testing. Never reuse its private key.

## 4. Privileged installation - explicit operator-run steps

**Only run after M1 security review and a successful independent backup of any existing local catalogue.** Check that nothing is already installed at the target path. Installation must not overwrite an existing binary, cert, key, catalogue or plist without a reviewed upgrade/rollback plan.

The operator needs a trusted loopback TLS certificate (IP SAN `127.0.0.1`), its private key, the staged binary and staged plist. Approve the certificate trust chain for the local Mac and the running browser/native client. Do not use insecure `curl -k` to bypass verification.

One-time installation structure (adjust only the service group to the verified account primary group):

```bash
BASE="/Library/Application Support/SupabaseBackupManager"
SERVICE_USER="_sbmbackup"
SERVICE_GROUP="$(id -gn "$SERVICE_USER")"
LABEL="org.example.supabasebackupmanager.daemon"
PLIST="/Library/LaunchDaemons/$LABEL.plist"

# Refuse an already loaded or installed service before proceeding.
test ! -e "$PLIST" || { echo "Existing launchd job: stop and review"; exit 1; }
test ! -e "$BASE/bin/backup-manager" || { echo "Existing binary: stop and review"; exit 1; }

sudo install -d -o root -g wheel -m 0755 "$BASE" "$BASE/bin" "$BASE/config"
sudo install -d -o "$SERVICE_USER" -g "$SERVICE_GROUP" -m 0700 "$BASE/private" "$BASE/logs"

# Staged absolute paths are provided by the operator. Do not use an unreviewed download.
sudo install -o root -g wheel -m 0555 "$HOME/Downloads/backup-manager-staged" "$BASE/bin/backup-manager"
sudo install -o root -g wheel -m 0644 /path/to/approved/loopback.crt "$BASE/config/loopback.crt"
sudo install -o "$SERVICE_USER" -g "$SERVICE_GROUP" -m 0600 /path/to/approved/loopback.key "$BASE/private/loopback.key"

# Required: audit the actual service user and installed file permissions.
python3 deploy/macos/launchd.py audit --config "$HOME/sbm-stage/deploy.json"

# Retain a verified, private copy of the staging file as installation evidence.
sudo install -o root -g wheel -m 0644 \
  "$HOME/sbm-stage/$LABEL.plist" "$PLIST"
sudo plutil -lint "$PLIST"
```

The audit will refuse missing paths, symlinks, incorrect owners/modes and an interactive/root service account. Review both the report and the actual installed plist before running `bootstrap`. To start:

```bash
sudo launchctl bootstrap system "$PLIST"
sudo launchctl print "system/$LABEL"
curl --fail --show-error --cacert "$BASE/config/loopback.crt" \
  "https://127.0.0.1:8765/api/v1/health"
```

This health endpoint reports **process health only**, not recoverability or approved identity. With the default locked configuration, `GET /api/v1/projects` must return **401**, including after the native app closes or when a GUI user signs out. Do not create production users to make this test pass. A forced crash test must take place **only with the disposable catalogue**, and must preserve the original service PID, exit status, restart count and logs as evidence.

## 5. Stop, uninstall, and rollback safely

Stopping is an explicit privileged action and is not equivalent to deleting data:

```bash
LABEL="org.example.supabasebackupmanager.daemon"
sudo launchctl print "system/$LABEL"
sudo launchctl bootout "system/$LABEL"
```

After confirming the service is stopped and no process is still listening, the operator may remove **only** the root-owned service plist, and then remove the binary if genuinely decommissioning:

```bash
sudo rm "/Library/LaunchDaemons/$LABEL.plist"
# Optional only after verification:
# sudo rm "/Library/Application Support/SupabaseBackupManager/bin/backup-manager"
```

**Never delete the catalogue, TLS key, private directory, snapshots, logs or service user as an implicit uninstall action.** Preserve them for recovery, retention and audit review. Restore a previous verified binary and plist only after stopping the daemon and ensuring catalogue schema compatibility; a SQLite downgrade is **not** automatic.

## 6. Mac mini acceptance matrix (manual evidence required)

| Test | Expected result | Evidence |
| --- | --- | --- |
| M1-A02 - app closed | Daemon remains healthy; app processes can exit | `launchctl print`, TLS health and PID after app quit |
| M1-A02 - GUI logged out | System daemon remains running, no user Keychain dependency | local login/logout record, PID and service logs |
| M1-A02 - FileVault/reboot | Service starts after boot/unlock and catalogue opens; no GUI dependency | boot event, uptime, `launchctl print`, verified TLS health |
| M1-A08 - SIGTERM | Clean exit with no stale listener or locked SQLite handles | launchd status, shutdown logs, subsequent restart |
| M1-A08 - crash recovery | launchd throttles and restarts; SQLite integrity/migrations pass | before/after PIDs, `launchctl print`, `PRAGMA integrity_check` |
| M1-A08 - Keychain locked | Backend is unaffected; native app cannot obtain a locked user's credentials until unlocked | dedicated test user, lock/unlock observation |
| M1-A10 - listener safety | Exact `127.0.0.1` or `::1`, TLS 1.3 only; no public/other host access | `lsof -nP -iTCP -sTCP:LISTEN`, verified TLS and failed non-loopback probes |
| M1-A10 - identity isolation | Wrong/missing bearer, disabled user, revoked token and cross-project access denied | approved IdP test accounts and request/audit evidence |
| M1-A10 - browser CSRF/origin | Same-origin and CSRF enforced; cross-origin write denied | browser tests and server audit events |
| M1-A10 - operational retention | Logs protected, rotated, no secrets, key renewal documented | owner/mode, disk-use, retention and rotation records |

**CI evidence today covers only a subset:** valid staged launchd plist, root execution guard, Go binary build on macOS, synthetic TLS health, local-only bind, default-deny project API, SQLite crash recovery and clean restart/shutdown. macOS CI may run Intel or Apple Silicon: capture `uname -m` and do not infer Apple Silicon testing from the runner name. It cannot verify actual privileged launchd start after reboot, GUI logout, FileVault state, UserName, Keychain unlock, or real account onboarding. Log rotation and a signed/notarised native app/installer also remain open for a production deployment.

**Stop conditions:** Do not proceed to real IdP logins or any production Supabase connection until M1-04 security acceptance, M1-07 machine acceptance, auditing and a dedicated architecture/security review are recorded. No production Supabase data is used in this milestone slice.

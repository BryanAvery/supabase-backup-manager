#!/bin/bash
# Build an unsigned, non-activating macOS preview installer.
# This script must run as a normal user on an Apple Silicon Mac.
# It never installs anything, calls sudo/launchctl or reads service secrets.
set -euo pipefail

if [[ $# -ne 5 ]]; then
  echo "usage: build_preview_pkg.sh APP_BUNDLE GO_BINARY WEB_DIST OUTPUT_DIR COMMIT_SHA" >&2
  exit 2
fi
APP="$1"
GO_BINARY="$2"
WEB_DIST="$3"
OUTPUT="$4"
COMMIT="$5"

if [[ "$(uname -s)" != "Darwin" || "$(uname -m)" != "arm64" || "$(id -u)" == "0" ]]; then
  echo "Refusing to package outside a non-root Apple Silicon macOS runner" >&2
  exit 2
fi
if [[ ! "$COMMIT" =~ ^[a-f0-9]{40}$ ]]; then
  echo "A full, literal Git commit SHA is required" >&2
  exit 2
fi
if [[ ! -d "$APP" || ! -f "$APP/Contents/Info.plist" || ! -x "$APP/Contents/MacOS/SupabaseBackupManager" ]]; then
  echo "Compiled SwiftUI application bundle is missing" >&2
  exit 2
fi
if [[ ! -x "$GO_BINARY" || ! -f "$WEB_DIST/index.html" || ! -d "$WEB_DIST/assets" ]]; then
  echo "Compiled Go service or browser production bundle is missing" >&2
  exit 2
fi
if [[ -e "$OUTPUT" && ! -d "$OUTPUT" ]]; then
  echo "Installer output must be a directory" >&2
  exit 2
fi

IDENTIFIER=$(/usr/libexec/PlistBuddy -c "Print :CFBundleIdentifier" "$APP/Contents/Info.plist")
SCHEME=$(/usr/libexec/PlistBuddy -c "Print :CFBundleURLTypes:0:CFBundleURLSchemes:0" "$APP/Contents/Info.plist")
if [[ "$IDENTIFIER" != "org.example.supabasebackupmanager" || "$SCHEME" != "$IDENTIFIER" ]]; then
  echo "Unexpected or unreviewed native app identity or callback scheme" >&2
  exit 2
fi
for binary in "$APP/Contents/MacOS/SupabaseBackupManager" "$GO_BINARY"; do
  if ! /usr/bin/lipo -archs "$binary" | grep -Eq '(^| )arm64( |$)'; then
    echo "Refusing a non-Apple-Silicon executable: $binary" >&2
    exit 2
  fi
done

SHORT_SHA="${COMMIT:0:12}"
PACKAGE="SupabaseBackupManager-macOS-arm64-UNSIGNED-PREVIEW.pkg"
ROOT="$(mktemp -d "${TMPDIR:-/tmp}/sbm-pkg.XXXXXX")"
trap 'rm -rf "$ROOT"' EXIT
PAYLOAD="$ROOT/payload"
APP_TARGET="$PAYLOAD/Applications/SupabaseBackupManager Preview.app"
STAGE="$PAYLOAD/Library/Application Support/SupabaseBackupManager/previews/$SHORT_SHA"
mkdir -p "$PAYLOAD/Applications" "$STAGE/bin" "$STAGE/web" "$STAGE/docs" "$OUTPUT"

/usr/bin/ditto "$APP" "$APP_TARGET"
/bin/cp "$GO_BINARY" "$STAGE/bin/backup-manager"
/usr/bin/ditto "$WEB_DIST" "$STAGE/web/dist"
# Only public development documentation, never private keys, actual client
# configuration, SQLite, launchd plists or bearer credentials.
cat > "$STAGE/docs/README-FIRST.txt" <<EOF
SUPABASE BACKUP MANAGER - UNSIGNED APPLE SILICON DEVELOPMENT PREVIEW
Build commit: $COMMIT

The installer places a read-only SwiftUI preview app in /Applications,
and unactivated Go service/browser build files in:
  /Library/Application Support/SupabaseBackupManager/previews/$SHORT_SHA

THIS INSTALLER DOES NOT:
  - create or start a system daemon, schedule launchd jobs or create a user;
  - provision accounts or trusted TLS/IdP settings;
  - touch the active catalogue, credentials, backups, keys or snapshots;
  - create/restore/connect to any production Supabase project.

The app remains locked until an approved OIDC identity provider is configured
on the Mac. Both the app and installer are UNSIGNED and NOT NOTARISED.
macOS Gatekeeper may prevent launching them. Do not disable security controls
or treat this preview package as general-purpose software distribution.

This package is for development and supervised acceptance testing only.
Review docs/implementation/M1-07_MACOS_LAUNCHD_OPERATIONS.md in the repo
before separately configuring a real system service.
EOF

# An ordinary installer payload, not a privileged daemon installer.
# No custom preinstall/postinstall scripts, LaunchDaemons files, credentials,
# live SQLite, or launchctl commands are packaged.
find "$PAYLOAD" -type l -print | grep -q . && {
  echo "Payload contains unexpected symlinks" >&2
  exit 2
}
if find "$PAYLOAD" -type f \( -name '*.key' -o -name '*.pem' -o -name '*.sqlite' -o -name '*.p12' -o -name '*.pfx' \) | grep -q .; then
  echo "Refusing private keys, identities or databases in preview package" >&2
  exit 2
fi
if find "$PAYLOAD" -type f -path '*/LaunchDaemons/*' | grep -q .; then
  echo "Launchd job must not be installed automatically" >&2
  exit 2
fi
/bin/chmod 0555 "$STAGE/bin/backup-manager"

PKG_PATH="$OUTPUT/$PACKAGE"
if [[ -e "$PKG_PATH" ]]; then
  echo "Refusing to overwrite an existing package: $PKG_PATH" >&2
  exit 2
fi
/usr/bin/pkgbuild \
  --root "$PAYLOAD" \
  --install-location / \
  --identifier org.example.supabasebackupmanager.preview \
  --version 0.1.0 \
  --ownership recommended \
  "$PKG_PATH"

# Verify the actual built package's recorded payload, not just the input tree.
FILES="$ROOT/payload-files.txt"
/usr/sbin/pkgutil --payload-files "$PKG_PATH" > "$FILES"
grep -Fq 'Applications/SupabaseBackupManager Preview.app/Contents/Info.plist' "$FILES"
grep -Fq "Support/SupabaseBackupManager/previews/$SHORT_SHA/bin/backup-manager" "$FILES"
grep -Fq "Support/SupabaseBackupManager/previews/$SHORT_SHA/web/dist/index.html" "$FILES"
if grep -Ei '(LaunchDaemons|/private/|\.key$|\.pem$|\.sqlite$|/Scripts/)' "$FILES"; then
  echo "Package contains a forbidden system-service, key or catalogue path" >&2
  exit 2
fi

/usr/bin/shasum -a 256 "$PKG_PATH" | /usr/bin/awk '{print $1 "  " $2}' > "$OUTPUT/SHA256SUMS"
cat > "$OUTPUT/READ-ME-FIRST.txt" <<EOF
Supabase Backup Manager - unsigned macOS arm64 preview installer
Git commit: $COMMIT
File: $PACKAGE
SHA-256: $(/usr/bin/shasum -a 256 "$PKG_PATH" | /usr/bin/awk '{print $1}')

This package is UNSIGNED, NOT NOTARISED, and is a DEVELOPMENT PREVIEW only.
macOS may block installation or app launch. Do not disable Gatekeeper or
other security controls to install it.

Payload:
- /Applications/SupabaseBackupManager Preview.app
- /Library/Application Support/SupabaseBackupManager/previews/$SHORT_SHA
  (unactivated Go service, browser bundle and README)

It does not install launchd, create service identities, start a listener,
configure authentication, change existing project databases, or touch live
Supabase data. Approved OIDC/TLS and a separately reviewed installation
are still required. This is NOT a production-ready signed installer.

Source and build evidence:
https://github.com/BryanAvery/supabase-backup-manager/commit/$COMMIT
EOF
cat > "$OUTPUT/build-manifest.json" <<EOF
{
  "commit": "$COMMIT",
  "platform": "macOS",
  "architecture": "arm64",
  "version": "0.1.0",
  "package": "$PACKAGE",
  "signed": false,
  "notarised": false,
  "daemonEnabledByInstaller": false,
  "containsLiveCredentials": false
}
EOF
echo "PASS: created unsigned preview installer at $PKG_PATH"

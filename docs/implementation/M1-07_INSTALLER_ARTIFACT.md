# Downloadable macOS installer from GitHub Actions

The repository publishes a **non-activating, unsigned Apple Silicon development preview** from `.github/workflows/m1-installer.yml`. The workflow uses macOS 15 on arm64, builds the SwiftUI app, Go backend and React browser frontend, checks their test suites, creates a standard macOS `.pkg` with `pkgbuild`, inspects the payload and uploads the package as a **GitHub Actions artifact**.

## Where to download it

1. Open the [GitHub Actions installer workflow](https://github.com/BryanAvery/supabase-backup-manager/actions/workflows/m1-installer.yml).
2. Open the latest successful run on `main`, or select **Run workflow** on `main`.
3. Under **Artifacts**, download `sbm-macos-arm64-unsigned-preview-<commit SHA>`.
4. Unzip the GitHub artifact. It contains:
   - `SupabaseBackupManager-macOS-arm64-UNSIGNED-PREVIEW.pkg`
   - `SHA256SUMS`
   - `READ-ME-FIRST.txt`
   - `build-manifest.json`
5. Check the SHA-256 checksum against `SHA256SUMS` before doing anything else:

```bash
cd /path/to/unzipped/artifact
shasum -a 256 -c SHA256SUMS
pkgutil --payload-files SupabaseBackupManager-macOS-arm64-UNSIGNED-PREVIEW.pkg
```

A run that fails compilation, package inspection, SHA-256 verification or a required test **will not upload a passing installation artifact**. Artifacts are retained for 30 days and require authorised access to the GitHub repository.

## What the package contains

- `/Applications/SupabaseBackupManager Preview.app`: the SwiftUI application shell. It defaults to locked and requires approved public OIDC configuration. Its example bundle identifier `org.example.supabasebackupmanager` is not authorised for real provider registration.
- `/Library/Application Support/SupabaseBackupManager/previews/<12-character commit>/bin/backup-manager`: a native arm64 Go backend executable, deliberately **not** in the active service location.
- `/Library/Application Support/SupabaseBackupManager/previews/<12-character commit>/web/dist`: compiled React static assets.
- A local, plain-text explanation of the safety boundary.

**The installer does not start or install the service.** It contains no launchd plist or installation scripts, creates no service account, does not use `sudo` or `launchctl`, does not configure TLS/IdP credentials, and never contacts a production Supabase project. The existing active catalogue, backups, private keys and logs remain outside its payload. It can overwrite a previous copy of the *preview application* if installed again, but does not target the active daemon paths.

## Important limitations

This development preview is **unsigned and not notarised**. macOS Gatekeeper may stop it from installing or launching. Do **not** disable Gatekeeper or remove quarantine protections just to run this build. The normal user-installable release will need an approved Apple Developer ID Application signing certificate, a Developer ID Installer certificate, notarisation and a release procedure. Those credentials are **not** present in the repository, so CI cannot honestly produce a generally trusted installer yet.

You can inspect the package or use it in a controlled test environment, with the risks understood. An unsigned `.pkg` is not evidence of security or integrity by itself; the SHA-256 checksum allows comparing the download to the build artifact, but it is not a substitute for trusted signing.

For actual service activation, account provisioning, hardware Keychain tests, post-reboot checks and rollback, use [M1-07 macOS operations runbook](M1-07_MACOS_LAUNCHD_OPERATIONS.md). These remain manual security-acceptance gates, not something the preview package performs.

## Developer notes

The underlying builder is `deploy/macos/build_preview_pkg.sh`. It refuses root, non-arm64 execution, an unexpected SwiftUI callback scheme, missing production build inputs, unexpected symlinks or keys/catalogues, and existing output package overwrites. It checks the actual package's payload list.

The workflow also verifies that the extracted component package has **no Installer scripts**, and checks both the installed app's `CFBundleIdentifier` and the actual arm64 binary architecture. The uploaded `build-manifest.json` records the exact Git commit, platform, architecture, unsigned status and lack of automatic service activation.

A GitHub Actions run is distinct from installing software on your own Mac. No installer is executed by this workflow.

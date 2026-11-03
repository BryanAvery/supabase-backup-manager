# Security policy

This project is in pre-implementation design. Do not use it to protect production systems.

Please report suspected vulnerabilities privately using GitHub's **Report a vulnerability** / private vulnerability reporting if enabled, rather than opening a public issue containing exploit details. Do not publish credentials or personal data in reports.

## Mandatory practices
- Never commit Supabase keys, database passwords, cloud tokens, signing keys, recovery keys or live backups.
- Use least-privilege credentials and protect local secrets using the macOS Keychain.
- Encrypt backup archives before upload and test independent key recovery.
- Enforce authentication, authorisation, auditing and re-authentication for destructive restore actions.
- Run secret scanning and dependency checks in CI.

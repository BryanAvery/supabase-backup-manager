-- M1-04: explicit instance-level privilege and local access-token revocation.
-- No administrator is seeded automatically. All grants are performed offline,
-- by the privileged catalogue operator, outside the HTTP interface.
CREATE TABLE instance_administrators (
  user_id TEXT PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
  granted_at TEXT NOT NULL,
  grant_reason TEXT NOT NULL,
  granted_by TEXT REFERENCES users(id)
);

-- SHA-256 digests of fully verified bearer tokens, never raw credentials.
CREATE TABLE revoked_access_tokens (
  token_sha256 TEXT PRIMARY KEY CHECK(length(token_sha256)=64),
  expires_at INTEGER NOT NULL,
  revoked_at TEXT NOT NULL
);
CREATE INDEX idx_revoked_access_expires ON revoked_access_tokens(expires_at);

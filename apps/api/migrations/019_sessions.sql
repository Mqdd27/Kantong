-- Login sessions, one row per signed-in device, so each can be listed and signed out.
-- id is HMAC(SESSION_SECRET, cookie token): a copy of this table cannot be replayed as cookies.
CREATE TABLE sessions (
  id TEXT PRIMARY KEY,
  user_agent TEXT NOT NULL DEFAULT '',
  ip TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now')),
  last_seen TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now')),
  expires_at INTEGER NOT NULL
);

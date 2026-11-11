-- optional AI chat through Hermes (on only when HERMES_API_URL is set)
CREATE TABLE chat_messages (
  id INTEGER PRIMARY KEY,
  role TEXT NOT NULL CHECK (role IN ('user', 'assistant')),
  text TEXT NOT NULL,
  model TEXT NOT NULL DEFAULT '', -- provider:model that answered (assistant rows)
  created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now'))
);

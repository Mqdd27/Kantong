-- separate chat threads so contexts don't mix
CREATE TABLE chat_sessions (
  id INTEGER PRIMARY KEY,
  title TEXT NOT NULL,
  updated_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now'))
);
ALTER TABLE chat_messages ADD COLUMN session_id INTEGER REFERENCES chat_sessions(id);
INSERT INTO chat_sessions (id, title) SELECT 1, 'Chat sebelumnya' WHERE EXISTS (SELECT 1 FROM chat_messages);
UPDATE chat_messages SET session_id = 1;
CREATE INDEX chat_messages_session ON chat_messages (session_id, id);

CREATE TABLE categories (
  id   INTEGER PRIMARY KEY,
  name TEXT NOT NULL,
  type TEXT NOT NULL CHECK (type IN ('in', 'out', 'transfer')),
  UNIQUE (name, type)
);

CREATE TABLE transactions (
  id           INTEGER PRIMARY KEY,
  occurred_at  TEXT    NOT NULL,                 -- RFC3339, always +07:00 so string order = time order
  amount       INTEGER NOT NULL CHECK (amount > 0),
  type         TEXT    NOT NULL CHECK (type IN ('in', 'out', 'transfer')),
  category     TEXT    NOT NULL,
  merchant     TEXT    NOT NULL DEFAULT '',
  account      TEXT    NOT NULL DEFAULT '',
  note         TEXT    NOT NULL DEFAULT '',
  source       TEXT    NOT NULL CHECK (source IN ('email', 'manual')),
  source_ref   TEXT    UNIQUE,
  confidence   REAL    CHECK (confidence BETWEEN 0 AND 1),
  needs_review INTEGER NOT NULL DEFAULT 0,
  created_at   TEXT    NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now')),
  updated_at   TEXT    NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now')),
  FOREIGN KEY (category, type) REFERENCES categories (name, type) ON UPDATE CASCADE
);
CREATE INDEX transactions_occurred_at ON transactions (occurred_at);

CREATE TABLE rules (
  id         INTEGER PRIMARY KEY,
  pattern    TEXT NOT NULL CHECK (pattern <> ''),
  match      TEXT NOT NULL DEFAULT 'contains' CHECK (match IN ('contains')),
  type       TEXT NOT NULL,
  category   TEXT NOT NULL,
  created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now')),
  FOREIGN KEY (category, type) REFERENCES categories (name, type) ON UPDATE CASCADE
);

INSERT INTO categories (name, type) VALUES
  ('Makan', 'out'), ('Jajan', 'out'), ('Transport', 'out'), ('Belanja', 'out'),
  ('Tagihan', 'out'), ('Investasi', 'out'), ('Hiburan', 'out'), ('Kesehatan', 'out'),
  ('Lainnya', 'out'),
  ('Gaji', 'in'), ('Transfer Masuk', 'in'), ('Refund', 'in'), ('Lainnya', 'in'),
  ('Transfer', 'transfer');

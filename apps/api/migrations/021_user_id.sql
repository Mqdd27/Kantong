-- Every row of personal data belongs to one account (user_id). Existing rows go to the first account.
-- Uniqueness that used to be global (source_ref, asset name, order_ref, budget key, deleted ref,
-- category name) is now per account, so two people can both have an asset called "BCA".
-- Built-in categories are copied per account (addUser), so a future migration that seeds a built-in
-- category must insert it for every user.
--
-- SQLite can't change UNIQUE/FK constraints in place, so the tables are rebuilt. New children
-- reference the *_new parents, which RENAME then rewrites to the final names; old children are
-- dropped before old parents so no ON DELETE CASCADE reaches the copied rows.

-- The first account must exist before rows point at it. Unclaimed (empty hash) until startup sets
-- its username and password from APP_USER/APP_PASSWORD; adduser before that gets id 2, not the data.
INSERT INTO users (id, username, pass_hash) SELECT 1, 'owner', '' WHERE NOT EXISTS (SELECT 1 FROM users);

CREATE TABLE categories_new (
  id        INTEGER PRIMARY KEY,
  user_id   INTEGER NOT NULL REFERENCES users (id),
  name      TEXT NOT NULL,
  type      TEXT NOT NULL CHECK (type IN ('in', 'out', 'transfer')),
  in_budget INTEGER NOT NULL DEFAULT 1,
  custom    INTEGER NOT NULL DEFAULT 0,
  UNIQUE (user_id, name, type)
);
INSERT INTO categories_new (id, user_id, name, type, in_budget, custom)
  SELECT id, (SELECT MIN(id) FROM users), name, type, in_budget, custom FROM categories;

CREATE TABLE transactions_new (
  id           INTEGER PRIMARY KEY,
  user_id      INTEGER NOT NULL REFERENCES users (id),
  occurred_at  TEXT    NOT NULL,                 -- RFC3339, always +07:00 so string order = time order
  amount       INTEGER NOT NULL CHECK (amount > 0),
  type         TEXT    NOT NULL CHECK (type IN ('in', 'out', 'transfer')),
  category     TEXT    NOT NULL,
  merchant     TEXT    NOT NULL DEFAULT '',
  account      TEXT    NOT NULL DEFAULT '',
  note         TEXT    NOT NULL DEFAULT '',
  source       TEXT    NOT NULL CHECK (source IN ('email', 'manual')),
  source_ref   TEXT,
  confidence   REAL    CHECK (confidence BETWEEN 0 AND 1),
  needs_review INTEGER NOT NULL DEFAULT 0,
  created_at   TEXT    NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now')),
  updated_at   TEXT    NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now')),
  currency     TEXT    NOT NULL DEFAULT 'IDR' CHECK (length(currency) = 3),
  orig_amount  REAL    CHECK (orig_amount > 0),
  to_account   TEXT    NOT NULL DEFAULT '',
  split_of     INTEGER,
  UNIQUE (user_id, source_ref),
  FOREIGN KEY (user_id, category, type) REFERENCES categories_new (user_id, name, type) ON UPDATE CASCADE
);
INSERT INTO transactions_new (id, user_id, occurred_at, amount, type, category, merchant, account, note, source, source_ref,
    confidence, needs_review, created_at, updated_at, currency, orig_amount, to_account, split_of)
  SELECT id, (SELECT MIN(id) FROM users), occurred_at, amount, type, category, merchant, account, note, source, source_ref,
    confidence, needs_review, created_at, updated_at, currency, orig_amount, to_account, split_of FROM transactions;

CREATE TABLE rules_new (
  id         INTEGER PRIMARY KEY,
  user_id    INTEGER NOT NULL REFERENCES users (id),
  pattern    TEXT NOT NULL CHECK (pattern <> ''),
  match      TEXT NOT NULL DEFAULT 'contains' CHECK (match IN ('contains')),
  type       TEXT NOT NULL,
  category   TEXT NOT NULL,
  created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now')),
  FOREIGN KEY (user_id, category, type) REFERENCES categories_new (user_id, name, type) ON UPDATE CASCADE
);
INSERT INTO rules_new (id, user_id, pattern, match, type, category, created_at)
  SELECT id, (SELECT MIN(id) FROM users), pattern, match, type, category, created_at FROM rules;

CREATE TABLE recurring_new (
  id          INTEGER PRIMARY KEY,
  user_id     INTEGER NOT NULL REFERENCES users (id),
  type        TEXT NOT NULL,
  category    TEXT NOT NULL,
  merchant    TEXT NOT NULL DEFAULT '',
  account     TEXT NOT NULL DEFAULT '',
  note        TEXT NOT NULL DEFAULT '',
  amount      INTEGER NOT NULL DEFAULT 0 CHECK (amount >= 0),
  currency    TEXT NOT NULL DEFAULT 'IDR',
  orig_amount REAL,
  freq        TEXT NOT NULL CHECK (freq IN ('weekdays', 'weekly', 'monthly', 'yearly')),
  day         INTEGER NOT NULL CHECK (day BETWEEN 1 AND 31),
  next_date   TEXT NOT NULL,
  auto        INTEGER NOT NULL DEFAULT 0,
  mask        INTEGER NOT NULL DEFAULT 0,
  to_account  TEXT NOT NULL DEFAULT '',
  FOREIGN KEY (user_id, category, type) REFERENCES categories_new (user_id, name, type)
);
INSERT INTO recurring_new (id, user_id, type, category, merchant, account, note, amount, currency, orig_amount, freq, day, next_date, auto, mask, to_account)
  SELECT id, (SELECT MIN(id) FROM users), type, category, merchant, account, note, amount, currency, orig_amount, freq, day, next_date, auto, mask, to_account FROM recurring;

CREATE TABLE assets_new (
  id          INTEGER PRIMARY KEY,
  user_id     INTEGER NOT NULL REFERENCES users (id),
  name        TEXT NOT NULL CHECK (name <> ''),
  kind        TEXT NOT NULL CHECK (kind IN ('bank', 'ewallet', 'forex', 'cash', 'stock', 'bond', 'sbn', 'mutual_fund', 'gold', 'crypto', 'other',
                                            'credit_card', 'paylater', 'loan')),
  currency    TEXT NOT NULL DEFAULT 'IDR' CHECK (length(currency) = 3),
  balance     REAL NOT NULL DEFAULT 0,
  invested    REAL,
  note        TEXT NOT NULL DEFAULT '',
  updated_at  TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now')),
  target      REAL CHECK (target > 0),
  target_date TEXT,
  return_pct  REAL,
  maturity    TEXT,
  payout      INTEGER NOT NULL DEFAULT 0 CHECK (payout IN (0, 1, 3, 6, 12)),
  face        REAL,
  due_day     INTEGER CHECK (due_day BETWEEN 1 AND 31),
  UNIQUE (user_id, name)
);
INSERT INTO assets_new (id, user_id, name, kind, currency, balance, invested, note, updated_at, target, target_date, return_pct, maturity, payout, face, due_day)
  SELECT id, (SELECT MIN(id) FROM users), name, kind, currency, balance, invested, note, updated_at, target, target_date, return_pct, maturity, payout, face, due_day FROM assets;

CREATE TABLE asset_orders_new (
  id          INTEGER PRIMARY KEY,
  user_id     INTEGER NOT NULL REFERENCES users (id),
  asset_id    INTEGER NOT NULL REFERENCES assets_new (id) ON DELETE CASCADE,
  side        TEXT NOT NULL CHECK (side IN ('buy', 'sell')),
  amount      REAL NOT NULL CHECK (amount > 0),  -- in the asset's currency
  status      TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'success', 'failed')),
  order_ref   TEXT NOT NULL,
  source_ref  TEXT,
  note        TEXT NOT NULL DEFAULT '',
  occurred_at TEXT NOT NULL,
  updated_at  TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now')),
  UNIQUE (user_id, order_ref)
);
INSERT INTO asset_orders_new (id, user_id, asset_id, side, amount, status, order_ref, source_ref, note, occurred_at, updated_at)
  SELECT id, (SELECT MIN(id) FROM users), asset_id, side, amount, status, order_ref, source_ref, note, occurred_at, updated_at FROM asset_orders;

CREATE TABLE asset_snapshots_new (
  user_id      INTEGER NOT NULL REFERENCES users (id),
  date         TEXT NOT NULL,
  total_idr    INTEGER NOT NULL,
  invested_idr INTEGER NOT NULL,
  PRIMARY KEY (user_id, date)
);
INSERT INTO asset_snapshots_new SELECT (SELECT MIN(id) FROM users), date, total_idr, invested_idr FROM asset_snapshots;

CREATE TABLE budgets_new (
  user_id  INTEGER NOT NULL REFERENCES users (id),
  month    TEXT NOT NULL CHECK (month GLOB '[0-9][0-9][0-9][0-9]-[0-9][0-9]'),
  category TEXT NOT NULL,
  amount   INTEGER NOT NULL CHECK (amount >= 0),
  PRIMARY KEY (user_id, category, month)
);
INSERT INTO budgets_new SELECT (SELECT MIN(id) FROM users), month, category, amount FROM budgets;

CREATE TABLE deleted_refs_new (
  user_id INTEGER NOT NULL REFERENCES users (id),
  ref     TEXT NOT NULL,
  PRIMARY KEY (user_id, ref)
);
INSERT INTO deleted_refs_new SELECT (SELECT MIN(id) FROM users), ref FROM deleted_refs;

DROP TABLE transactions;
DROP TABLE rules;
DROP TABLE recurring;
DROP TABLE asset_orders;
DROP TABLE categories;
DROP TABLE assets;
DROP TABLE asset_snapshots;
DROP TABLE budgets;
DROP TABLE deleted_refs;

ALTER TABLE categories_new RENAME TO categories;
ALTER TABLE assets_new RENAME TO assets;
ALTER TABLE transactions_new RENAME TO transactions;
ALTER TABLE rules_new RENAME TO rules;
ALTER TABLE recurring_new RENAME TO recurring;
ALTER TABLE asset_orders_new RENAME TO asset_orders;
ALTER TABLE asset_snapshots_new RENAME TO asset_snapshots;
ALTER TABLE budgets_new RENAME TO budgets;
ALTER TABLE deleted_refs_new RENAME TO deleted_refs;

CREATE INDEX transactions_occurred_at ON transactions (user_id, occurred_at);
CREATE INDEX transactions_split_of ON transactions (split_of) WHERE split_of IS NOT NULL;
CREATE INDEX asset_orders_status ON asset_orders (status);

-- chat_messages hang off chat_sessions, so the session carries the owner
ALTER TABLE chat_sessions ADD COLUMN user_id INTEGER REFERENCES users (id);
UPDATE chat_sessions SET user_id = (SELECT MIN(id) FROM users);

-- per-account API tokens for Hermes and scripts; only the SHA-256 of the token is stored
CREATE TABLE api_tokens (
  id         INTEGER PRIMARY KEY,
  user_id    INTEGER NOT NULL REFERENCES users (id) ON DELETE CASCADE,
  hash       TEXT NOT NULL UNIQUE,
  created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now'))
);

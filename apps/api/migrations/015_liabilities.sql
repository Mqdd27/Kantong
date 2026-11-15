-- Liabilities: credit cards, paylater and loans are assets with a negative balance (= what is owed),
-- so net worth is the plain sum and spending/payments booked to the account move the debt.
-- SQLite can't alter a CHECK, so rebuild the table. Dropping it cascades to asset_orders
-- (foreign_keys is ON and can't be turned off inside the migration's transaction), so keep a copy.
CREATE TEMP TABLE orders_bak AS SELECT * FROM asset_orders;

CREATE TABLE assets_new (
  id          INTEGER PRIMARY KEY,
  name        TEXT NOT NULL UNIQUE CHECK (name <> ''),
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
  face        REAL
);
INSERT INTO assets_new (id, name, kind, currency, balance, invested, note, updated_at, target, target_date, return_pct, maturity, payout, face)
  SELECT id, name, kind, currency, balance, invested, note, updated_at, target, target_date, return_pct, maturity, payout, face FROM assets;
DROP TABLE assets;
ALTER TABLE assets_new RENAME TO assets;

INSERT INTO asset_orders SELECT * FROM orders_bak;
DROP TABLE orders_bak;

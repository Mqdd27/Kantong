-- Multi-currency transactions: `amount` stays the IDR value used for reports;
-- `orig_amount` is the amount in `currency` when that is not IDR.
ALTER TABLE transactions ADD COLUMN currency TEXT NOT NULL DEFAULT 'IDR' CHECK (length(currency) = 3);
ALTER TABLE transactions ADD COLUMN orig_amount REAL CHECK (orig_amount > 0);

-- IDR value of one unit of `currency`, cached per requested date.
CREATE TABLE fx_rates (
  date     TEXT NOT NULL,
  currency TEXT NOT NULL,
  idr      REAL NOT NULL CHECK (idr > 0),
  PRIMARY KEY (date, currency)
);

CREATE TABLE assets (
  id         INTEGER PRIMARY KEY,
  name       TEXT NOT NULL UNIQUE CHECK (name <> ''),
  kind       TEXT NOT NULL CHECK (kind IN ('bank', 'ewallet', 'forex', 'cash', 'stock', 'bond', 'sbn', 'mutual_fund', 'gold', 'crypto', 'other')),
  currency   TEXT NOT NULL DEFAULT 'IDR' CHECK (length(currency) = 3),
  balance    REAL NOT NULL DEFAULT 0,  -- current value in `currency`
  invested   REAL,                     -- cost basis in `currency`, optional
  note       TEXT NOT NULL DEFAULT '',
  updated_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now'))
);

-- Investment orders reported by email: pending until a success/failed email arrives.
CREATE TABLE asset_orders (
  id          INTEGER PRIMARY KEY,
  asset_id    INTEGER NOT NULL REFERENCES assets (id) ON DELETE CASCADE,
  side        TEXT NOT NULL CHECK (side IN ('buy', 'sell')),
  amount      REAL NOT NULL CHECK (amount > 0),  -- in the asset's currency
  status      TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'success', 'failed')),
  order_ref   TEXT NOT NULL UNIQUE,
  source_ref  TEXT,
  note        TEXT NOT NULL DEFAULT '',
  occurred_at TEXT NOT NULL,
  updated_at  TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now'))
);
CREATE INDEX asset_orders_status ON asset_orders (status);

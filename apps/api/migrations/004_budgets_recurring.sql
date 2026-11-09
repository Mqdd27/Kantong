-- Spending limits. category '' = overall limit. A row applies from `month` until a
-- later row for the same category; amount 0 removes the limit from that month on.
CREATE TABLE budgets (
  month    TEXT NOT NULL CHECK (month GLOB '[0-9][0-9][0-9][0-9]-[0-9][0-9]'),
  category TEXT NOT NULL,
  amount   INTEGER NOT NULL CHECK (amount >= 0),
  PRIMARY KEY (category, month)
);

-- Out-categories that are saving/investing, not spending: kept out of the overall budget.
ALTER TABLE categories ADD COLUMN in_budget INTEGER NOT NULL DEFAULT 1;
UPDATE categories SET in_budget = 0 WHERE type = 'out' AND name = 'Investasi';

-- Bills, subscriptions, salary... `day` is the anchor day of month (keeps "31st" stable).
CREATE TABLE recurring (
  id          INTEGER PRIMARY KEY,
  type        TEXT NOT NULL,
  category    TEXT NOT NULL,
  merchant    TEXT NOT NULL DEFAULT '',
  account     TEXT NOT NULL DEFAULT '',
  note        TEXT NOT NULL DEFAULT '',
  amount      INTEGER NOT NULL DEFAULT 0 CHECK (amount >= 0), -- IDR; 0 = convert orig_amount when posting
  currency    TEXT NOT NULL DEFAULT 'IDR',
  orig_amount REAL,
  freq        TEXT NOT NULL CHECK (freq IN ('weekly', 'monthly', 'yearly')),
  day         INTEGER NOT NULL CHECK (day BETWEEN 1 AND 31),
  next_date   TEXT NOT NULL,             -- YYYY-MM-DD
  auto        INTEGER NOT NULL DEFAULT 0, -- 1 = post automatically on the date
  FOREIGN KEY (category, type) REFERENCES categories (name, type)
);

-- Savings goals on assets.
ALTER TABLE assets ADD COLUMN target REAL CHECK (target > 0);
ALTER TABLE assets ADD COLUMN target_date TEXT;

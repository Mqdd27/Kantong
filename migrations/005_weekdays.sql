-- Allow freq = 'weekdays' (Mon–Fri). SQLite can't alter a CHECK, so rebuild the table.
CREATE TABLE recurring_new (
  id          INTEGER PRIMARY KEY,
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
  FOREIGN KEY (category, type) REFERENCES categories (name, type)
);
INSERT INTO recurring_new SELECT * FROM recurring;
DROP TABLE recurring;
ALTER TABLE recurring_new RENAME TO recurring;

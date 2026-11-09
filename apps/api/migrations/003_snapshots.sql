-- One row per day: net worth and total contributed capital, both in IDR.
CREATE TABLE asset_snapshots (
  date         TEXT PRIMARY KEY,
  total_idr    INTEGER NOT NULL,
  invested_idr INTEGER NOT NULL
);

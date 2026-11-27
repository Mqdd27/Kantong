-- Kantong bersama (docs/shared-kantong.md): one asset row used by several accounts.
-- assets.user_id is the current holder (the creator, or whoever took over); a member's transactions
-- keep their own user_id and count toward the kantong when their account name matches it.
-- No assets rebuild: a kantong name is kept unique among its members by the app, so
-- UNIQUE (user_id, name) never conflicts.
CREATE TABLE shared_assets (
  id         INTEGER PRIMARY KEY,
  asset_id   INTEGER NOT NULL UNIQUE REFERENCES assets (id) ON DELETE CASCADE,
  created_by INTEGER NOT NULL REFERENCES users (id),
  -- transactions recorded before this don't belong to the kantong, so a member's older entries
  -- under the same account name stay private
  created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now'))
);
CREATE TABLE shared_members (
  shared_id INTEGER NOT NULL REFERENCES shared_assets (id) ON DELETE CASCADE,
  user_id   INTEGER NOT NULL REFERENCES users (id) ON DELETE CASCADE,
  role      TEXT NOT NULL CHECK (role IN ('owner', 'member')),
  joined_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now')),
  PRIMARY KEY (shared_id, user_id)
);
CREATE INDEX shared_members_user ON shared_members (user_id);

-- who entered the row; user_id can move to the kantong holder when its author leaves
ALTER TABLE transactions ADD COLUMN created_by INTEGER REFERENCES users (id) ON DELETE SET NULL;
UPDATE transactions SET created_by = user_id;

-- kantong_tx is the one definition of which transactions belong to which kantong:
-- recorded by a current member, after the kantong was created, booked to or transferred into its name.
CREATE VIEW kantong_tx AS
SELECT s.id AS shared_id, s.asset_id, t.id AS tx_id
FROM shared_assets s
JOIN assets a ON a.id = s.asset_id
JOIN shared_members m ON m.shared_id = s.id
JOIN transactions t ON t.user_id = m.user_id AND t.created_at >= s.created_at
  AND (lower(trim(t.account)) = lower(a.name) OR lower(t.to_account) = lower(a.name));

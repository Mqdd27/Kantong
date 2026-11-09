-- Destination account of a transfer, so both accounts' asset balances follow it.
ALTER TABLE transactions ADD COLUMN to_account TEXT NOT NULL DEFAULT '';

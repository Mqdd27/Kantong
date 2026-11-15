-- Split transactions: one payment spread over several categories. Every part, the original row
-- included, has split_of = the original's id; the original keeps the source_ref so the email stays known.
ALTER TABLE transactions ADD COLUMN split_of INTEGER;
CREATE INDEX transactions_split_of ON transactions (split_of) WHERE split_of IS NOT NULL;

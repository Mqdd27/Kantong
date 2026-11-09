-- Which weekdays (weekly: bit 0 = Sunday … bit 6 = Saturday) or months (monthly: bit 0 = Jan … bit 11 = Dec)
-- a recurring item falls on. 0 = every 7 days / every month from next_date.
ALTER TABLE recurring ADD COLUMN mask INTEGER NOT NULL DEFAULT 0;
UPDATE recurring SET freq = 'weekly', mask = 62 WHERE freq = 'weekdays'; -- Mon–Fri

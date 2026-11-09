-- How often a bond/SBN pays out its return, in months: 1, 3, 6, 12; 0 = no coupon (return accrues into the value).
ALTER TABLE assets ADD COLUMN payout INTEGER NOT NULL DEFAULT 0 CHECK (payout IN (0, 1, 3, 6, 12));

-- Expected return: coupon/yield for bonds & SBN, projected return for funds etc. (% per year), and bond maturity.
ALTER TABLE assets ADD COLUMN return_pct REAL;
ALTER TABLE assets ADD COLUMN maturity TEXT; -- YYYY-MM-DD

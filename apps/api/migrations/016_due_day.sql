-- Debt due day (1-31): the app shows the next due date and flags a missed payment.
ALTER TABLE assets ADD COLUMN due_day INTEGER CHECK (due_day BETWEEN 1 AND 31);
-- Recurring transfers into an own account, e.g. a monthly paylater installment paid from the bank.
ALTER TABLE recurring ADD COLUMN to_account TEXT NOT NULL DEFAULT '';

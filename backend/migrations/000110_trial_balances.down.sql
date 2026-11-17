DROP TABLE IF EXISTS trial_balance_lines;
DROP INDEX IF EXISTS idx_account_plan_imports_trial_balance;
ALTER TABLE account_plan_imports DROP CONSTRAINT IF EXISTS chk_account_plan_imports_period;
ALTER TABLE account_plan_imports DROP COLUMN IF EXISTS period;

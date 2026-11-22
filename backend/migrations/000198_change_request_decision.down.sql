ALTER TABLE change_requests
    DROP CONSTRAINT IF EXISTS chk_change_requests_target_period,
    DROP CONSTRAINT IF EXISTS chk_change_requests_decision,
    DROP COLUMN IF EXISTS target_period,
    DROP COLUMN IF EXISTS decision;

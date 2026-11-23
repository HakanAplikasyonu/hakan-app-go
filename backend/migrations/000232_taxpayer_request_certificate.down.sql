ALTER TABLE taxpayer_onboarding_requests
    DROP COLUMN IF EXISTS activity_code,
    DROP COLUMN IF EXISTS address,
    DROP COLUMN IF EXISTS tax_office;

ALTER TABLE taxpayers
    DROP COLUMN IF EXISTS activity_code,
    DROP COLUMN IF EXISTS address;

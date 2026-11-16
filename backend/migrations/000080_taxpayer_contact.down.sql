ALTER TABLE taxpayers
    DROP CONSTRAINT IF EXISTS chk_taxpayers_contact_email,
    DROP CONSTRAINT IF EXISTS chk_taxpayers_contact_phone,
    DROP COLUMN IF EXISTS contact_email,
    DROP COLUMN IF EXISTS contact_phone;

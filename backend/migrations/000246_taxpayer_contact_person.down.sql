ALTER TABLE taxpayers
    DROP CONSTRAINT IF EXISTS chk_taxpayers_contact_person_email,
    DROP CONSTRAINT IF EXISTS chk_taxpayers_contact_person_phone,
    DROP CONSTRAINT IF EXISTS chk_taxpayers_contact_person_name,
    DROP COLUMN IF EXISTS contact_person_email,
    DROP COLUMN IF EXISTS contact_person_phone,
    DROP COLUMN IF EXISTS contact_person_name;

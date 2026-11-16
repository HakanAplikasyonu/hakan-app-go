-- Mükellef iletişim bilgisi (issue #238): müşavir mobilde "ara" ve "e-posta"
-- için. İsteğe bağlı; boşsa mükellef üyelerinin e-postası kullanılır.
ALTER TABLE taxpayers
    ADD COLUMN contact_phone TEXT NULL,
    ADD COLUMN contact_email CITEXT NULL,
    ADD CONSTRAINT chk_taxpayers_contact_phone CHECK (contact_phone IS NULL OR contact_phone ~ '^\+?[0-9 ()-]{7,20}$'),
    ADD CONSTRAINT chk_taxpayers_contact_email CHECK (contact_email IS NULL OR char_length(contact_email) <= 254);

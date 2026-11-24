-- Mükellefin yetkili kişisi (#780): muhasebe ofisinin muhatabı çoğu zaman
-- mükellef hesabının sahibi değildir (muhasebeci, mali işler sorumlusu).
-- Ofis adını, telefonunu ve e-postasını serbest metin olarak tutar. Hepsi
-- isteğe bağlı; boşsa ekranlar eskisi gibi hesap sahibini gösterir.
-- taxpayers tablosunun mevcut RLS'i (rls_taxpayer_identity) yeni sütunları da
-- kapsar; ek politika gerekmez.
ALTER TABLE taxpayers
    ADD COLUMN contact_person_name  TEXT   NULL,
    ADD COLUMN contact_person_phone TEXT   NULL,
    ADD COLUMN contact_person_email CITEXT NULL,
    ADD CONSTRAINT chk_taxpayers_contact_person_name
        CHECK (contact_person_name IS NULL OR char_length(contact_person_name) BETWEEN 1 AND 120),
    ADD CONSTRAINT chk_taxpayers_contact_person_phone
        CHECK (contact_person_phone IS NULL OR contact_person_phone ~ '^\+?[0-9 ()-]{7,20}$'),
    ADD CONSTRAINT chk_taxpayers_contact_person_email
        CHECK (contact_person_email IS NULL OR char_length(contact_person_email) <= 254);

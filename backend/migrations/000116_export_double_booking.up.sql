-- Luca çıkışında çift kayıt koruması (#466).
--
-- Luca e-belgeleri entegratörden kendisi de çekiyor (İşNet NetteFatura,
-- TÜRMOB e-Belge, GİB e-Belge). Portalın fiş dosyasıyla aynı fatura Luca'da
-- ikinci kez işlenebilir. Üç parça:
--
-- 1. Ayar: Luca'nın e-faturaları kendisi çektiği entegratörler. Ofis geneli
--    (office_export_settings) ve mükellef bazında (export_settings; NULL =
--    ofis ayarı geçerli).
-- 2. Belge izi: dışa aktarılan her fişin kaynak belgesi (ETTN) ve girdiği
--    dosya (export_batch_documents). Dışa aktarım geçmişinde belge → dosya.
-- 3. Onay: aynı belge daha önce bir dosyada çıktıysa ya da Luca'nın kendisi
--    çekiyorsa fiş varsayılan olarak dosyaya girmez; müşavir onaylarsa
--    onaylanan uyarı kodları, kim ve ne zaman bu satırda tutulur.

-- Ofis iş ürünü: organization_id kapsamlı, ofis başına tek satır.
CREATE TABLE office_export_settings (
    id                     BIGSERIAL    PRIMARY KEY,
    organization_id        BIGINT       NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    -- Luca'nın e-faturaları kendisi çektiği entegratörler: isnet (İşNet
    -- NetteFatura), turmob (TÜRMOB e-Belge), gib (GİB e-Belge).
    luca_pull_integrators  TEXT[]       NOT NULL DEFAULT '{}',
    updated_by             BIGINT       NULL REFERENCES users (id) ON DELETE SET NULL,
    created_at             TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at             TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_office_export_settings_org UNIQUE (organization_id),
    CONSTRAINT chk_office_export_settings_integrators
        CHECK (luca_pull_integrators <@ ARRAY['isnet', 'turmob', 'gib']::text[])
);

CREATE TRIGGER trg_office_export_settings_set_updated_at
    BEFORE UPDATE ON office_export_settings FOR EACH ROW EXECUTE FUNCTION set_updated_at();

SELECT rls_apply_org('office_export_settings'::regclass);

-- Mükellef bazında: NULL = ofis ayarı, boş dizi = Luca bu mükellefin hiçbir
-- faturasını çekmiyor.
ALTER TABLE export_settings ADD COLUMN luca_pull_integrators TEXT[] NULL;
ALTER TABLE export_settings ADD CONSTRAINT chk_export_settings_integrators
    CHECK (luca_pull_integrators IS NULL OR luca_pull_integrators <@ ARRAY['isnet', 'turmob', 'gib']::text[]);

-- Bir çıktıdaki fişin kaynak belgesi ve girdiği dosya. Bir fiş birden çok
-- faturadan gelebilir (bir satır = bir belge anahtarı).
CREATE TABLE export_batch_documents (
    id                BIGSERIAL    PRIMARY KEY,
    organization_id   BIGINT       NOT NULL,
    engagement_id     BIGINT       NOT NULL,
    export_batch_id   BIGINT       NOT NULL,
    journal_draft_id  BIGINT       NOT NULL,
    -- invoice (fatura ya da kalemi). Belge anahtarı: faturanın ETTN'si
    -- (küçük harf).
    source_kind       TEXT         NOT NULL,
    document_key      TEXT         NOT NULL,
    -- Fatura mükellef verisidir (taxpayer kapsamı); burada yalnızca uuid'si
    -- tutulur, yabancı anahtar yok.
    invoice_uuid      UUID         NULL,
    document_no       TEXT         NULL,
    document_date     DATE         NULL,
    -- Fişin yazıldığı dosya (export_batches.files[].name).
    file_name         TEXT         NOT NULL,
    -- Müşavirin bu çıktı için onayladığı uyarı kodları
    -- (DOCUMENT_ALREADY_EXPORTED, PROGRAM_PULLS_DOCUMENT); boş = uyarı yoktu.
    confirmed_codes   TEXT[]       NOT NULL DEFAULT '{}',
    confirmed_by      BIGINT       NULL REFERENCES users (id) ON DELETE SET NULL,
    confirmed_at      TIMESTAMPTZ  NULL,
    created_at        TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    CONSTRAINT fk_export_batch_documents_batch
        FOREIGN KEY (engagement_id, export_batch_id) REFERENCES export_batches (engagement_id, id) ON DELETE CASCADE,
    CONSTRAINT fk_export_batch_documents_draft
        FOREIGN KEY (engagement_id, journal_draft_id) REFERENCES journal_drafts (engagement_id, id) ON DELETE RESTRICT,
    CONSTRAINT uq_export_batch_documents_doc UNIQUE (export_batch_id, journal_draft_id, document_key),
    CONSTRAINT chk_export_batch_documents_kind CHECK (source_kind IN ('invoice')),
    CONSTRAINT chk_export_batch_documents_key CHECK (document_key <> '' AND document_key = lower(document_key)),
    CONSTRAINT chk_export_batch_documents_confirmed CHECK ((cardinality(confirmed_codes) = 0) = (confirmed_at IS NULL)),
    CONSTRAINT chk_export_batch_documents_codes
        CHECK (confirmed_codes <@ ARRAY['DOCUMENT_ALREADY_EXPORTED', 'PROGRAM_PULLS_DOCUMENT']::text[])
);

-- Ön kontrol: "bu ETTN bu mükellefte daha önce çıktı mı".
CREATE INDEX idx_export_batch_documents_key ON export_batch_documents (organization_id, engagement_id, document_key);
CREATE INDEX idx_export_batch_documents_batch ON export_batch_documents (engagement_id, export_batch_id);
CREATE INDEX idx_export_batch_documents_draft ON export_batch_documents (engagement_id, journal_draft_id);

SELECT rls_apply_engagement('export_batch_documents'::regclass);

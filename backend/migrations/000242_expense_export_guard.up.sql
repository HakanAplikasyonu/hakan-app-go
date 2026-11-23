-- Masrafta çift aktarım koruması (#780), faturalardaki korumanın (#466)
-- aynısı. Belge anahtarı "exp:<masraf uuid>".
--
-- 1) Yevmiye dışa aktarımı: onaylı masrafın yevmiye fişi Luca'ya gidince
--    masraf da export_batch_documents'a belge olarak yazılır
--    (source_kind = 'expense').
-- 2) Masraflar ekranındaki "Luca'ya aktar (Excel)" dosyası yevmiye taslağı
--    ve dışa aktarma kaydı açmaz; giden her masraf burada tutulur: hangi
--    çıktı (export_uuid, dosya), kim, ne zaman.
--
-- İkisi birlikte okunur: bir masraf hangi yoldan gitmiş olursa olsun ikinci
-- çıktıda varsayılan olarak dışarıda kalır; müşavir o çıktı için "yeniden
-- dahil et" derse girer ve onay kodu (DOCUMENT_ALREADY_EXPORTED) yazılır.
ALTER TABLE export_batch_documents DROP CONSTRAINT chk_export_batch_documents_kind;
ALTER TABLE export_batch_documents ADD CONSTRAINT chk_export_batch_documents_kind
    CHECK (source_kind IN ('invoice', 'expense'));
-- Masraf mükellef verisidir; yalnızca uuid'si tutulur, yabancı anahtar yok.
ALTER TABLE export_batch_documents ADD COLUMN expense_uuid UUID NULL;

CREATE TABLE expense_export_documents (
    id               BIGSERIAL    PRIMARY KEY,
    organization_id  BIGINT       NOT NULL,
    engagement_id    BIGINT       NOT NULL,
    -- Bir indirme = bir çıktı; dosyaları aynı export_uuid'yi taşır.
    export_uuid      UUID         NOT NULL,
    driver           TEXT         NOT NULL,
    period           CHAR(7)      NOT NULL,
    file_name        TEXT         NOT NULL,
    document_key     TEXT         NOT NULL,
    expense_uuid     UUID         NOT NULL,
    document_no      TEXT         NULL,
    document_date    DATE         NULL,
    confirmed_codes  TEXT[]       NOT NULL DEFAULT '{}',
    exported_by      BIGINT       NULL REFERENCES users (id) ON DELETE SET NULL,
    created_at       TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    CONSTRAINT fk_expense_export_documents_engagement
        FOREIGN KEY (organization_id, engagement_id) REFERENCES engagements (organization_id, id) ON DELETE CASCADE,
    CONSTRAINT uq_expense_export_documents_doc UNIQUE (export_uuid, document_key),
    CONSTRAINT chk_expense_export_documents_key CHECK (document_key <> '' AND document_key = lower(document_key)),
    CONSTRAINT chk_expense_export_documents_period CHECK (period ~ '^[0-9]{4}-(0[1-9]|1[0-2])$'),
    CONSTRAINT chk_expense_export_documents_codes
        CHECK (confirmed_codes <@ ARRAY['DOCUMENT_ALREADY_EXPORTED']::text[])
);

-- Ön kontrol: "bu masraf bu mükellefte daha önce çıktı mı".
CREATE INDEX idx_expense_export_documents_key ON expense_export_documents (organization_id, engagement_id, document_key);
CREATE INDEX idx_expense_export_documents_exported_by ON expense_export_documents (exported_by);

SELECT rls_apply_engagement('expense_export_documents'::regclass);

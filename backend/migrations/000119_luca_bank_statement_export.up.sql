-- Luca banka ekstresi CSV çıktısı (#465).
--
-- bank_statement_exports: bir banka hesabının (cash_boxes) bir tarih
-- aralığındaki hareketlerinin Luca "Banka Ekstreleri" CSV'si olarak
-- indirilmesi; şablon sürümü, tutar düzeni, ayraç, dosya özeti.
-- bank_statement_export_items: dosyaya giren hareketler. Aynı hareket
-- ikinci kez dışa aktarılırsa banka ekranı bunu işaretler.
--
-- Ofis iş ürünü: organization_id + engagement_id, engagement kapsamlı RLS.
-- Banka hesabı ve hareketler mükellef verisidir: taxpayer_id ile yabancı
-- anahtar.

CREATE TABLE bank_statement_exports (
    id                BIGSERIAL    PRIMARY KEY,
    uuid              UUID         NOT NULL DEFAULT gen_random_uuid(),
    organization_id   BIGINT       NOT NULL,
    engagement_id     BIGINT       NOT NULL,
    taxpayer_id       BIGINT       NOT NULL REFERENCES taxpayers (id) ON DELETE RESTRICT,
    cash_box_id       BIGINT       NOT NULL,
    target            TEXT         NOT NULL DEFAULT 'luca',
    date_from         DATE         NOT NULL,
    date_to           DATE         NOT NULL,
    template_version  TEXT         NOT NULL,
    template_verified BOOLEAN      NOT NULL,
    -- signed | two_columns; ayraç ve tırnak dosyada kullanılan değer.
    amount_layout     TEXT         NOT NULL,
    delimiter         TEXT         NOT NULL,
    quote             TEXT         NOT NULL,
    file_name         TEXT         NOT NULL,
    sha256            CHAR(64)     NOT NULL,
    size              INT          NOT NULL,
    line_count        INT          NOT NULL,
    -- Bu dosyada daha önce dışa aktarılmış kaç hareket vardı.
    repeated_count    INT          NOT NULL DEFAULT 0,
    created_by        BIGINT       NULL REFERENCES users (id) ON DELETE SET NULL,
    created_at        TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_bank_statement_exports_uuid UNIQUE (uuid),
    CONSTRAINT uq_bank_statement_exports_eng_id UNIQUE (engagement_id, id),
    CONSTRAINT fk_bank_statement_exports_engagement
        FOREIGN KEY (organization_id, engagement_id) REFERENCES engagements (organization_id, id) ON DELETE CASCADE,
    CONSTRAINT fk_bank_statement_exports_cash_box
        FOREIGN KEY (taxpayer_id, cash_box_id) REFERENCES cash_boxes (taxpayer_id, id) ON DELETE CASCADE,
    CONSTRAINT chk_bank_statement_exports_range CHECK (date_from <= date_to),
    CONSTRAINT chk_bank_statement_exports_layout CHECK (amount_layout IN ('signed', 'two_columns')),
    CONSTRAINT chk_bank_statement_exports_target CHECK (target ~ '^[a-z][a-z0-9_]*$'),
    CONSTRAINT chk_bank_statement_exports_counts CHECK (size >= 0 AND line_count >= 0 AND repeated_count >= 0 AND repeated_count <= line_count)
);

CREATE INDEX idx_bank_statement_exports_list ON bank_statement_exports (organization_id, engagement_id, created_at DESC);
CREATE INDEX idx_bank_statement_exports_cash_box ON bank_statement_exports (taxpayer_id, cash_box_id);

SELECT rls_apply_engagement('bank_statement_exports'::regclass);

CREATE TABLE bank_statement_export_items (
    id                      BIGSERIAL    PRIMARY KEY,
    organization_id         BIGINT       NOT NULL,
    engagement_id           BIGINT       NOT NULL,
    bank_statement_export_id BIGINT      NOT NULL,
    taxpayer_id             BIGINT       NOT NULL,
    bank_transaction_id     BIGINT       NOT NULL,
    -- Bu dosyadan önce aynı hareketin kaç kez dışa aktarıldığı (0 = ilk).
    previous_exports        INT          NOT NULL DEFAULT 0,
    created_at              TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    CONSTRAINT fk_bank_statement_export_items_export
        FOREIGN KEY (engagement_id, bank_statement_export_id) REFERENCES bank_statement_exports (engagement_id, id) ON DELETE CASCADE,
    CONSTRAINT fk_bank_statement_export_items_transaction
        FOREIGN KEY (taxpayer_id, bank_transaction_id) REFERENCES bank_transactions (taxpayer_id, id) ON DELETE CASCADE,
    CONSTRAINT uq_bank_statement_export_items_pair UNIQUE (bank_statement_export_id, bank_transaction_id),
    CONSTRAINT chk_bank_statement_export_items_previous CHECK (previous_exports >= 0)
);

-- "Bu hareket daha önce çıktı mı": engagement + hareket.
CREATE INDEX idx_bank_statement_export_items_tx ON bank_statement_export_items (organization_id, engagement_id, bank_transaction_id);
CREATE INDEX idx_bank_statement_export_items_fk_tx ON bank_statement_export_items (taxpayer_id, bank_transaction_id);

SELECT rls_apply_engagement('bank_statement_export_items'::regclass);

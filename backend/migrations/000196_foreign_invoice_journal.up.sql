-- Yabancı hizmet faturasından fiş taslağı (#780, KDV2 B). Onaylı faturadan
-- uygulama fiş taslağı üretir (gider / 360 KDV2 / 191 / 360 stopaj / 320),
-- fiş diğer fişlerle Luca'ya aktarılır.
--
-- Hesap kodları ofis ayarıdır: gider hesabı işlem türüne göre
-- (foreign_service_kind_settings.expense_account), diğerleri
-- foreign_invoice_settings sütunlarında. NULL: Go'daki hazır varsayılan
-- (modules/foreigninvoices/journal.go).
ALTER TABLE foreign_service_kind_settings ADD COLUMN expense_account TEXT NULL;

ALTER TABLE foreign_invoice_settings
    ADD COLUMN kdv2_account             TEXT NULL,
    ADD COLUMN stopaj_account           TEXT NULL,
    ADD COLUMN deductible_vat_account   TEXT NULL,
    ADD COLUMN counter_account          TEXT NULL,
    ADD COLUMN rounding_income_account  TEXT NULL,
    ADD COLUMN rounding_expense_account TEXT NULL;

-- Bir iş (engagement) bir yabancı faturadan en fazla bir fiş üretir. Fiş
-- silinirse bağ da gider (fatura yeniden fişlenebilir).
CREATE TABLE foreign_invoice_journal_links (
    id                  BIGSERIAL    PRIMARY KEY,
    organization_id     BIGINT       NOT NULL,
    engagement_id       BIGINT       NOT NULL,
    foreign_invoice_id  BIGINT       NOT NULL REFERENCES foreign_service_invoices (id) ON DELETE RESTRICT,
    journal_draft_id    BIGINT       NOT NULL,
    created_by          BIGINT       NULL REFERENCES users (id) ON DELETE SET NULL,
    created_at          TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_foreign_invoice_journal_links_invoice UNIQUE (engagement_id, foreign_invoice_id),
    CONSTRAINT fk_foreign_invoice_journal_links_engagement
        FOREIGN KEY (organization_id, engagement_id) REFERENCES engagements (organization_id, id) ON DELETE CASCADE,
    CONSTRAINT fk_foreign_invoice_journal_links_journal
        FOREIGN KEY (engagement_id, journal_draft_id) REFERENCES journal_drafts (engagement_id, id) ON DELETE CASCADE
);

CREATE INDEX idx_foreign_invoice_journal_links_org_engagement ON foreign_invoice_journal_links (organization_id, engagement_id);
CREATE INDEX idx_foreign_invoice_journal_links_invoice ON foreign_invoice_journal_links (foreign_invoice_id);
CREATE INDEX idx_foreign_invoice_journal_links_journal ON foreign_invoice_journal_links (engagement_id, journal_draft_id);
CREATE INDEX idx_foreign_invoice_journal_links_created_by ON foreign_invoice_journal_links (created_by);

SELECT rls_apply_engagement('foreign_invoice_journal_links'::regclass);

-- Fiş satırının kaynağı yabancı fatura olabilir.
ALTER TABLE journal_draft_lines DROP CONSTRAINT chk_journal_draft_lines_source_kind;
ALTER TABLE journal_draft_lines ADD CONSTRAINT chk_journal_draft_lines_source_kind
    CHECK (source_kind IS NULL OR source_kind IN ('invoice', 'invoice_item', 'bank_transaction', 'receipt', 'manual', 'payroll', 'foreign_invoice'));

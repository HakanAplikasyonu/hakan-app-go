-- Kaynak finansal işlemleri (issue #385, docs/kaynak 01 §4.9, 09 §5):
-- tahsilat, ödeme, gelir/gider ve perakende satış ödemeleri. Alan adları ve
-- değerler Easytrade'inkilerdir (payment_method_type, transaction_type,
-- transaction_flow). direction ⊕ türetilir (in = para girişi, out = çıkış).
-- invoice_id: referenceable_type = Invoice olan hareketin portaldaki
-- faturası (eşleştirme / mutabakat girdisi). Ham veri alan şifrelemesiyle
-- mühürlenir (secrets.PurposeFinancialTxRawPayload).
CREATE TABLE source_financial_transactions (
    id                      BIGSERIAL      PRIMARY KEY,
    uuid                    UUID           NOT NULL DEFAULT gen_random_uuid(),
    taxpayer_id             BIGINT         NOT NULL REFERENCES taxpayers (id) ON DELETE RESTRICT,
    source_system           TEXT           NOT NULL,
    source_id               TEXT           NOT NULL,
    capture_method          TEXT           NOT NULL DEFAULT 'api',
    transaction_date        DATE           NOT NULL,
    due_date                DATE           NULL,
    period                  CHAR(7)        NOT NULL,
    transaction_type        TEXT           NOT NULL DEFAULT '',
    transaction_flow        TEXT           NOT NULL DEFAULT '',
    direction               TEXT           NOT NULL DEFAULT 'unknown',
    payment_method_type     TEXT           NOT NULL DEFAULT 'unknown',
    amount                  NUMERIC(18,4)  NOT NULL,
    currency                CHAR(3)        NOT NULL DEFAULT 'TRY',
    exchange_rate           NUMERIC(18,6)  NULL,
    amount_try              NUMERIC(18,4)  NOT NULL,
    description             TEXT           NOT NULL DEFAULT '',
    reference_document_no   TEXT           NULL,
    status                  TEXT           NOT NULL DEFAULT '',
    customer_source_id      TEXT           NULL,
    customer_name           TEXT           NULL,
    customer_tax_number     VARCHAR(11)    NULL,
    cash_box_source_id      TEXT           NULL,
    cash_box_name           TEXT           NULL,
    cash_box_type           TEXT           NULL,
    referenceable_type      TEXT           NULL,
    referenceable_id        TEXT           NULL,
    invoice_id              BIGINT         NULL,
    raw_payload             JSONB          NOT NULL DEFAULT '{}'::jsonb,
    content_hash            CHAR(64)       NULL,
    created_at              TIMESTAMPTZ    NOT NULL DEFAULT NOW(),
    updated_at              TIMESTAMPTZ    NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_source_financial_transactions_uuid UNIQUE (uuid),
    CONSTRAINT uq_source_financial_transactions_taxpayer_id UNIQUE (taxpayer_id, id),
    CONSTRAINT uq_source_financial_transactions_source UNIQUE (taxpayer_id, source_system, source_id),
    CONSTRAINT fk_source_financial_transactions_invoice
        FOREIGN KEY (taxpayer_id, invoice_id) REFERENCES invoices (taxpayer_id, id) ON DELETE SET NULL (invoice_id),
    CONSTRAINT chk_source_financial_transactions_capture CHECK (capture_method IN ('api', 'excel_import', 'manual')),
    CONSTRAINT chk_source_financial_transactions_direction CHECK (direction IN ('in', 'out', 'unknown')),
    CONSTRAINT chk_source_financial_transactions_payment CHECK (payment_method_type IN (
        'cash', 'bank_transfer', 'credit_card', 'check', 'promissory_note', 'credit', 'unknown')),
    CONSTRAINT chk_source_financial_transactions_amount CHECK (amount >= 0 AND amount_try >= 0),
    CONSTRAINT chk_source_financial_transactions_currency CHECK (currency ~ '^[A-Z]{3}$'),
    CONSTRAINT chk_source_financial_transactions_period CHECK (period ~ '^[0-9]{4}-(0[1-9]|1[0-2])$')
);

CREATE INDEX idx_source_financial_transactions_period ON source_financial_transactions (taxpayer_id, period);
CREATE INDEX idx_source_financial_transactions_customer ON source_financial_transactions (taxpayer_id, customer_tax_number)
    WHERE customer_tax_number IS NOT NULL;
CREATE INDEX idx_source_financial_transactions_invoice ON source_financial_transactions (taxpayer_id, invoice_id)
    WHERE invoice_id IS NOT NULL;

CREATE TRIGGER trg_source_financial_transactions_set_updated_at
    BEFORE UPDATE ON source_financial_transactions FOR EACH ROW EXECUTE FUNCTION set_updated_at();

SELECT rls_apply_taxpayer('source_financial_transactions');

-- Banka hareketi → mizan hesabı (issue #383). Her hareket için karşı hesap
-- önerisi (102 karşısında 120 / 320 / 335 / 360 / 770 ...), güven skoru ve
-- kaynağı; inceleme kuyruğunda (suggestions, kind bank_account) onaylanınca
-- approved olur, onaylananlardan journal_drafts (source_kind
-- bank_transaction) üretilir. Ofisin iş ürünüdür: organization_id +
-- engagement_id; hareket mükellefin kaynak verisidir (taxpayer_id).

-- Mükellefin banka hesabının (cash box) mizandaki hesabı (102.01 ...).
-- Satırı olmayan hesap "102" ile fişlenir.
CREATE TABLE bank_ledger_accounts (
    id               BIGSERIAL    PRIMARY KEY,
    organization_id  BIGINT       NOT NULL,
    engagement_id    BIGINT       NOT NULL,
    taxpayer_id      BIGINT       NOT NULL REFERENCES taxpayers (id) ON DELETE RESTRICT,
    cash_box_id      BIGINT       NOT NULL,
    account_code     VARCHAR(64)  NOT NULL,
    updated_by       BIGINT       NULL REFERENCES users (id) ON DELETE SET NULL,
    created_at       TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at       TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_bank_ledger_accounts_box UNIQUE (engagement_id, cash_box_id),
    CONSTRAINT fk_bank_ledger_accounts_engagement
        FOREIGN KEY (organization_id, engagement_id) REFERENCES engagements (organization_id, id) ON DELETE CASCADE,
    CONSTRAINT fk_bank_ledger_accounts_cash_box
        FOREIGN KEY (taxpayer_id, cash_box_id) REFERENCES cash_boxes (taxpayer_id, id) ON DELETE CASCADE,
    CONSTRAINT chk_bank_ledger_accounts_code CHECK (account_code ~ '^[0-9]{3}(\.[0-9A-Za-z]+)*$')
);

CREATE TRIGGER trg_bank_ledger_accounts_set_updated_at
    BEFORE UPDATE ON bank_ledger_accounts FOR EACH ROW EXECUTE FUNCTION set_updated_at();

SELECT rls_apply_engagement('bank_ledger_accounts'::regclass);

CREATE TABLE bank_transaction_postings (
    id                   BIGSERIAL     PRIMARY KEY,
    organization_id      BIGINT        NOT NULL,
    engagement_id        BIGINT        NOT NULL,
    taxpayer_id          BIGINT        NOT NULL REFERENCES taxpayers (id) ON DELETE RESTRICT,
    bank_transaction_id  BIGINT        NOT NULL,
    -- Karşı hesap (banka hesabının karşısı).
    account_code         VARCHAR(64)   NOT NULL,
    account_name         TEXT          NOT NULL DEFAULT '',
    -- suggested: öneri incelemede; approved: müşavir ya da eşik onayladı.
    status               TEXT          NOT NULL DEFAULT 'suggested',
    confidence           NUMERIC(5,4)  NOT NULL,
    -- alias | counterparty_account | invoice_match | kind_default | manual
    source               TEXT          NOT NULL,
    -- Gerekçe (G12): eşleşen takma ad, cari ya da işlem türü.
    reason               TEXT          NOT NULL DEFAULT '',
    suggestion_id        BIGINT        NULL,
    -- Onaylanan hareketin girdiği fiş taslağı (journal_drafts.uuid).
    journal_draft_uuid   UUID          NULL,
    approved_by          BIGINT        NULL REFERENCES users (id) ON DELETE SET NULL,
    approved_at          TIMESTAMPTZ   NULL,
    created_at           TIMESTAMPTZ   NOT NULL DEFAULT NOW(),
    updated_at           TIMESTAMPTZ   NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_bank_transaction_postings_txn UNIQUE (engagement_id, bank_transaction_id),
    CONSTRAINT fk_bank_transaction_postings_engagement
        FOREIGN KEY (organization_id, engagement_id) REFERENCES engagements (organization_id, id) ON DELETE CASCADE,
    CONSTRAINT fk_bank_transaction_postings_transaction
        FOREIGN KEY (taxpayer_id, bank_transaction_id) REFERENCES bank_transactions (taxpayer_id, id) ON DELETE CASCADE,
    CONSTRAINT chk_bank_transaction_postings_code CHECK (account_code ~ '^[0-9]{3}(\.[0-9A-Za-z]+)*$'),
    CONSTRAINT chk_bank_transaction_postings_status CHECK (status IN ('suggested', 'approved')),
    CONSTRAINT chk_bank_transaction_postings_source CHECK (source IN ('alias', 'counterparty_account', 'invoice_match', 'kind_default', 'manual')),
    CONSTRAINT chk_bank_transaction_postings_confidence CHECK (confidence >= 0 AND confidence <= 1),
    CONSTRAINT chk_bank_transaction_postings_approved CHECK ((status = 'approved') = (approved_at IS NOT NULL))
);

CREATE INDEX idx_bank_transaction_postings_status ON bank_transaction_postings (organization_id, engagement_id, status);

CREATE TRIGGER trg_bank_transaction_postings_set_updated_at
    BEFORE UPDATE ON bank_transaction_postings FOR EACH ROW EXECUTE FUNCTION set_updated_at();

SELECT rls_apply_engagement('bank_transaction_postings'::regclass);

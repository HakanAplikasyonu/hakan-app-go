-- Banka–fatura eşleşmesi (issue #21, 21 §6.2): bank_transaction_match.
-- Mükellefin kaynak verisi (taxpayer_tables.txt). Bir hareket birden çok
-- faturayı kapatabilir (toplu ödeme); bir fatura birden çok hareketle
-- (taksit) kapanabilir.
CREATE TABLE bank_transaction_matches (
    id                   BIGSERIAL      PRIMARY KEY,
    uuid                 UUID           NOT NULL DEFAULT gen_random_uuid(),
    taxpayer_id          BIGINT         NOT NULL REFERENCES taxpayers (id) ON DELETE RESTRICT,
    bank_transaction_id  BIGINT         NOT NULL,
    target_type          TEXT           NOT NULL,
    target_id            BIGINT         NOT NULL,
    amount               NUMERIC(18,4)  NOT NULL,
    score                SMALLINT       NOT NULL,
    method               TEXT           NOT NULL,
    -- Puan bileşenleri ve gerekçe (G12).
    evidence             JSONB          NOT NULL DEFAULT '{}'::jsonb,
    matched_by           BIGINT         NULL REFERENCES users (id) ON DELETE SET NULL,
    matched_at           TIMESTAMPTZ    NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_bank_transaction_matches_uuid UNIQUE (uuid),
    CONSTRAINT uq_bank_transaction_matches_target UNIQUE (taxpayer_id, bank_transaction_id, target_type, target_id),
    CONSTRAINT fk_bank_transaction_matches_transaction
        FOREIGN KEY (taxpayer_id, bank_transaction_id) REFERENCES bank_transactions (taxpayer_id, id) ON DELETE CASCADE,
    CONSTRAINT chk_bank_transaction_matches_target_type CHECK (target_type IN ('invoice')),
    CONSTRAINT chk_bank_transaction_matches_method CHECK (method IN ('auto', 'suggested_accepted', 'manual')),
    CONSTRAINT chk_bank_transaction_matches_score CHECK (score >= 0 AND score <= 200),
    CONSTRAINT chk_bank_transaction_matches_amount CHECK (amount > 0)
);

CREATE INDEX idx_bank_transaction_matches_target ON bank_transaction_matches (taxpayer_id, target_type, target_id);

-- RLS (#6, DATABASE_RULES.md): mükellef kapsamı.
SELECT rls_apply_taxpayer('bank_transaction_matches');

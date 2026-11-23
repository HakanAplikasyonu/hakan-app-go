-- Eksik ekstre takibi (Oğuz'un geri bildirimi): mükellefin kullanımdaki her
-- banka hesabı için kapanmış her ay "ekstre geldi" ya da "eksik" sayılır.
-- Ekstre geldi = o ay o hesapta hareket ya da gün sonu bakiyesi var. Hareketi
-- olmayan bir ayda müşavir "ekstre geldi (hareket yok)" diye işaretler; bu
-- tablo o işaretlerdir. Ofisin kararıdır: engagement kapsamında tutulur.
CREATE TABLE bank_statement_marks (
    id               BIGSERIAL    PRIMARY KEY,
    organization_id  BIGINT       NOT NULL,
    engagement_id    BIGINT       NOT NULL,
    taxpayer_id      BIGINT       NOT NULL,
    cash_box_id      BIGINT       NOT NULL,
    period           TEXT         NOT NULL,
    created_by       BIGINT       NULL REFERENCES users (id) ON DELETE SET NULL,
    created_at       TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    CONSTRAINT fk_bank_statement_marks_engagement
        FOREIGN KEY (organization_id, engagement_id) REFERENCES engagements (organization_id, id) ON DELETE CASCADE,
    CONSTRAINT fk_bank_statement_marks_cash_box
        FOREIGN KEY (taxpayer_id, cash_box_id) REFERENCES cash_boxes (taxpayer_id, id) ON DELETE CASCADE,
    CONSTRAINT uq_bank_statement_marks UNIQUE (engagement_id, cash_box_id, period),
    CONSTRAINT chk_bank_statement_marks_period CHECK (period ~ '^[0-9]{4}-(0[1-9]|1[0-2])$')
);

CREATE INDEX idx_bank_statement_marks_list ON bank_statement_marks (organization_id, engagement_id, period);
CREATE INDEX idx_bank_statement_marks_cash_box ON bank_statement_marks (taxpayer_id, cash_box_id);

SELECT rls_apply_engagement('bank_statement_marks'::regclass);

-- Aylık durum hesabın o ayki hareketine bakar.
CREATE INDEX IF NOT EXISTS idx_bank_transactions_cash_box_period ON bank_transactions (taxpayer_id, cash_box_id, period);

-- Banka bakiyesi ile muhasebe (102) bakiyesi farkı (#780 C): farkın her
-- nedeni için ofisin kararı. "explained": neden açıklandı, fişe gerek yok;
-- "corrected": düzeltme fişi taslağı kesildi (journal_draft_uuid). Kim, ne
-- zaman, hangi karşı hesap ve açıklama burada tutulur. Ofisin iş ürünüdür:
-- organization_id + engagement_id; banka hesabı mükellefin kaynak verisidir.
CREATE TABLE bank_difference_resolutions (
    id                  BIGSERIAL      PRIMARY KEY,
    uuid                UUID           NOT NULL DEFAULT gen_random_uuid(),
    organization_id     BIGINT         NOT NULL,
    engagement_id       BIGINT         NOT NULL,
    taxpayer_id         BIGINT         NOT NULL REFERENCES taxpayers (id) ON DELETE RESTRICT,
    cash_box_id         BIGINT         NOT NULL,
    period              CHAR(7)        NOT NULL,
    -- opening | unbooked | other_account | manual_lines | gaps | duplicates | unexplained
    reason              TEXT           NOT NULL,
    action              TEXT           NOT NULL,
    amount              NUMERIC(18,4)  NULL,
    counter_account     VARCHAR(64)    NULL,
    explanation         TEXT           NOT NULL,
    journal_draft_uuid  UUID           NULL,
    created_by          BIGINT         NULL REFERENCES users (id) ON DELETE SET NULL,
    created_at          TIMESTAMPTZ    NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_bank_difference_resolutions_uuid UNIQUE (uuid),
    CONSTRAINT fk_bank_difference_resolutions_engagement
        FOREIGN KEY (organization_id, engagement_id) REFERENCES engagements (organization_id, id) ON DELETE CASCADE,
    CONSTRAINT fk_bank_difference_resolutions_cash_box
        FOREIGN KEY (taxpayer_id, cash_box_id) REFERENCES cash_boxes (taxpayer_id, id) ON DELETE CASCADE,
    CONSTRAINT chk_bank_difference_resolutions_period CHECK (period ~ '^[0-9]{4}-(0[1-9]|1[0-2])$'),
    CONSTRAINT chk_bank_difference_resolutions_reason CHECK (reason IN (
        'opening', 'unbooked', 'other_account', 'manual_lines', 'gaps', 'duplicates', 'unexplained')),
    CONSTRAINT chk_bank_difference_resolutions_action CHECK (action IN ('explained', 'corrected')),
    CONSTRAINT chk_bank_difference_resolutions_explanation CHECK (btrim(explanation) <> ''),
    CONSTRAINT chk_bank_difference_resolutions_corrected CHECK (
        (action = 'corrected') = (journal_draft_uuid IS NOT NULL AND amount IS NOT NULL AND counter_account IS NOT NULL)),
    CONSTRAINT chk_bank_difference_resolutions_code CHECK (
        counter_account IS NULL OR counter_account ~ '^[0-9]{3}(\.[0-9A-Za-z]+)*$')
);

CREATE INDEX idx_bank_difference_resolutions_period
    ON bank_difference_resolutions (organization_id, engagement_id, period, cash_box_id);
CREATE INDEX idx_bank_difference_resolutions_cash_box
    ON bank_difference_resolutions (taxpayer_id, cash_box_id);
-- Bir neden bir dönemde bir kez "açıklandı" işaretlenir.
CREATE UNIQUE INDEX uq_bank_difference_resolutions_explained
    ON bank_difference_resolutions (engagement_id, cash_box_id, period, reason) WHERE action = 'explained';

-- RLS (issue #6): engagement kapsamlı.
SELECT rls_apply_engagement('bank_difference_resolutions'::regclass);

-- Mükellefe giden KDV bildirimi ve gönderim kanıtı (#780 B, 21.11 ofis
-- görüşmesi). Ofis mükellefe "bu ay şu an KDV ödemeniz çıkıyor / çıkmıyor"
-- bilgisini gönderir; o anki tahmin burada dondurulur (KDV nöbetçisi
-- sonradan değişen durumu buna göre bulur). Her kanalın gönderildi / okundu
-- zamanı notifications satırından okunur (notification_uuids).
CREATE TABLE vat_notices (
    id                 BIGSERIAL      PRIMARY KEY,
    uuid               UUID           NOT NULL DEFAULT gen_random_uuid(),
    organization_id    BIGINT         NOT NULL,
    engagement_id      BIGINT         NOT NULL,
    taxpayer_id        BIGINT         NOT NULL REFERENCES taxpayers (id) ON DELETE RESTRICT,
    period             CHAR(7)        NOT NULL,
    -- payment: ödeme çıkıyor; no_payment: çıkmıyor (devreden var ya da sıfır).
    outcome            TEXT           NOT NULL,
    output_vat         NUMERIC(18,2)  NOT NULL,
    input_vat          NUMERIC(18,2)  NOT NULL,
    carried_in         NUMERIC(18,2)  NOT NULL,
    payable            NUMERIC(18,2)  NOT NULL,
    carry_out          NUMERIC(18,2)  NOT NULL,
    due_date           DATE           NULL,
    message            TEXT           NULL,
    notification_uuids UUID[]         NOT NULL DEFAULT '{}',
    sent_by            BIGINT         NULL REFERENCES users (id) ON DELETE SET NULL,
    created_at         TIMESTAMPTZ    NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_vat_notices_uuid UNIQUE (uuid),
    CONSTRAINT fk_vat_notices_engagement
        FOREIGN KEY (organization_id, engagement_id) REFERENCES engagements (organization_id, id) ON DELETE CASCADE,
    CONSTRAINT chk_vat_notices_outcome CHECK (outcome IN ('payment', 'no_payment')),
    CONSTRAINT chk_vat_notices_period CHECK (period ~ '^[0-9]{4}-(0[1-9]|1[0-2])$')
);

CREATE INDEX idx_vat_notices_engagement ON vat_notices (organization_id, engagement_id, period, created_at DESC);
CREATE INDEX idx_vat_notices_taxpayer ON vat_notices (taxpayer_id);
CREATE INDEX idx_vat_notices_sent_by ON vat_notices (sent_by);

SELECT rls_apply_engagement('vat_notices'::regclass);

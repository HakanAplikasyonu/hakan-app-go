-- Mükellef raporu gönderimleri (issue #386): ofisin mükellefe e-postayla
-- gönderdiği aylık / yıllık rapor. Satır, istek anındaki raporun anlık
-- görüntüsünü (report) taşır; worker taraması PDF'i bu görüntüden üretip
-- gönderir (outbox deseni: kuyruğa yaz, tarama gönderir, yeniden dener).
-- Gönderim geçmişi aynı tablodur.
CREATE TABLE report_deliveries (
    id               BIGSERIAL    PRIMARY KEY,
    uuid             UUID         NOT NULL DEFAULT gen_random_uuid(),
    organization_id  BIGINT       NOT NULL,
    engagement_id    BIGINT       NOT NULL,
    taxpayer_id      BIGINT       NOT NULL REFERENCES taxpayers (id) ON DELETE RESTRICT,
    -- month | year; period YYYY-MM ya da YYYY.
    kind             TEXT         NOT NULL,
    period           TEXT         NOT NULL,
    recipient        TEXT         NOT NULL,
    -- manual (tek tık), bulk (seçili mükellefler), scheduled (ay sonu).
    origin           TEXT         NOT NULL DEFAULT 'manual',
    status           TEXT         NOT NULL DEFAULT 'queued',
    attempts         INTEGER      NOT NULL DEFAULT 0,
    next_attempt_at  TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    last_error       TEXT         NULL,
    -- Raporun anlık görüntüsü (TaxpayerReport) ve gönderen ofisin adı.
    report           JSONB        NOT NULL,
    office_name      TEXT         NOT NULL DEFAULT '',
    requested_by     BIGINT       NULL REFERENCES users (id) ON DELETE SET NULL,
    sent_at          TIMESTAMPTZ  NULL,
    created_at       TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at       TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_report_deliveries_uuid UNIQUE (uuid),
    CONSTRAINT fk_report_deliveries_engagement
        FOREIGN KEY (organization_id, engagement_id) REFERENCES engagements (organization_id, id) ON DELETE CASCADE,
    CONSTRAINT chk_report_deliveries_kind CHECK (kind IN ('month', 'year')),
    CONSTRAINT chk_report_deliveries_period CHECK (
        (kind = 'month' AND period ~ '^[0-9]{4}-(0[1-9]|1[0-2])$') OR (kind = 'year' AND period ~ '^[0-9]{4}$')),
    CONSTRAINT chk_report_deliveries_origin CHECK (origin IN ('manual', 'bulk', 'scheduled')),
    CONSTRAINT chk_report_deliveries_status CHECK (status IN ('queued', 'sending', 'sent', 'failed')),
    CONSTRAINT chk_report_deliveries_report CHECK (jsonb_typeof(report) = 'object')
);

CREATE INDEX idx_report_deliveries_list ON report_deliveries (organization_id, engagement_id, created_at DESC);
CREATE INDEX idx_report_deliveries_office ON report_deliveries (organization_id, created_at DESC);
CREATE INDEX idx_report_deliveries_due ON report_deliveries (next_attempt_at) WHERE status IN ('queued', 'sending');

CREATE TRIGGER trg_report_deliveries_set_updated_at
    BEFORE UPDATE ON report_deliveries FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- Ofis + engagement kapsamı; FORCE ve politika şablondan (000086).
SELECT rls_apply_engagement('report_deliveries'::regclass);

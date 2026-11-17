-- Zamanlanmış mükellef raporu (issue #386): ofis bir mükellef için aylık
-- raporun her ay kendiliğinden gönderilmesini açar. Ay kapanınca, sonraki
-- ayın day_of_month gününden itibaren önceki ayın raporu bir kez
-- report_deliveries kuyruğuna (origin = scheduled) yazılır; last_period
-- aynı ayın ikinci kez gönderilmesini önler.
CREATE TABLE report_schedules (
    organization_id  BIGINT       NOT NULL,
    engagement_id    BIGINT       NOT NULL,
    taxpayer_id      BIGINT       NOT NULL REFERENCES taxpayers (id) ON DELETE RESTRICT,
    enabled          BOOLEAN      NOT NULL DEFAULT true,
    day_of_month     INTEGER      NOT NULL DEFAULT 1,
    -- Boşsa mükellefin kayıtlı e-postası.
    recipient        TEXT         NULL,
    last_period      CHAR(7)      NULL,
    last_error       TEXT         NULL,
    updated_by       BIGINT       NULL REFERENCES users (id) ON DELETE SET NULL,
    created_at       TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at       TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    CONSTRAINT pk_report_schedules PRIMARY KEY (organization_id, engagement_id),
    CONSTRAINT fk_report_schedules_engagement
        FOREIGN KEY (organization_id, engagement_id) REFERENCES engagements (organization_id, id) ON DELETE CASCADE,
    CONSTRAINT chk_report_schedules_day CHECK (day_of_month BETWEEN 1 AND 28),
    CONSTRAINT chk_report_schedules_period CHECK (last_period IS NULL OR last_period ~ '^[0-9]{4}-(0[1-9]|1[0-2])$')
);

CREATE INDEX idx_report_schedules_due ON report_schedules (day_of_month) WHERE enabled;

CREATE TRIGGER trg_report_schedules_set_updated_at
    BEFORE UPDATE ON report_schedules FOR EACH ROW EXECUTE FUNCTION set_updated_at();

SELECT rls_apply_engagement('report_schedules'::regclass);

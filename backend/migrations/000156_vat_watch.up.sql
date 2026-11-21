-- KDV nöbetçisi (#780 B, 21.11 ofis görüşmesi): her gün her mükellefin bu
-- ayki KDV'si hesaplanır. Mükellefe "ödeme çıkmayacak" denmişken yeni bir
-- fatura durumu değiştirirse ofise "KDV durumu değişti" uyarısı gider;
-- ödenecek KDV ofisin eşiğini geçerse "KDV eşiği aşıldı". Üretilen her olay
-- burada bir kez yazılır (dedupe_key): aynı mükellefe aynı dönemde aynı
-- olay ikinci kez gitmez. Olaylar outbox üzerinden iletişim merkezinin
-- (K2) otomasyon kurallarına da düşer.
ALTER TABLE vat_tracking_settings ADD COLUMN watch_enabled BOOLEAN NOT NULL DEFAULT TRUE;

CREATE TABLE vat_watch_events (
    id               BIGSERIAL    PRIMARY KEY,
    uuid             UUID         NOT NULL DEFAULT gen_random_uuid(),
    organization_id  BIGINT       NOT NULL,
    engagement_id    BIGINT       NOT NULL,
    period           CHAR(7)      NOT NULL,
    event            TEXT         NOT NULL,
    dedupe_key       TEXT         NOT NULL,
    payload          JSONB        NOT NULL DEFAULT '{}'::jsonb,
    created_at       TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_vat_watch_events_uuid UNIQUE (uuid),
    CONSTRAINT uq_vat_watch_events_dedupe UNIQUE (organization_id, dedupe_key),
    CONSTRAINT fk_vat_watch_events_engagement
        FOREIGN KEY (organization_id, engagement_id) REFERENCES engagements (organization_id, id) ON DELETE CASCADE,
    CONSTRAINT chk_vat_watch_events_event CHECK (event IN ('vat.threshold_exceeded', 'vat.status_changed', 'vat.report_ready')),
    CONSTRAINT chk_vat_watch_events_period CHECK (period ~ '^[0-9]{4}-(0[1-9]|1[0-2])$')
);

CREATE INDEX idx_vat_watch_events_engagement ON vat_watch_events (organization_id, engagement_id, created_at DESC);

SELECT rls_apply_engagement('vat_watch_events'::regclass);

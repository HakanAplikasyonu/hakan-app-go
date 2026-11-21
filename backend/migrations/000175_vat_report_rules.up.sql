-- KDV raporu kuralları (#780 B, Oğuz'un isteği): ayın 29'u sabit değil.
-- Ofis bir gün listesi tanımlar (örn. 15'i ara rapor, 25'i, 29'u) ya da
-- "ay sonundan X gün önce". Her kuralda kime gideceği (yalnız ofis, ofis
-- onayıyla mükellef, doğrudan mükellef) ve hangi mükellefler (tümü, grup,
-- seçili, yalnız ödemesi çıkanlar) seçilir. Kural çalışınca her mükellef
-- için "vat.report_ready" olayı üretilir (vat_watch_events, bir kez).
CREATE TABLE vat_report_rules (
    id                BIGSERIAL    PRIMARY KEY,
    uuid              UUID         NOT NULL DEFAULT gen_random_uuid(),
    organization_id   BIGINT       NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    -- day: ayın `day`'i (ay kısaysa son gün); before_month_end: ay sonundan `days_before` gün önce.
    trigger_kind      TEXT         NOT NULL,
    day               INTEGER      NULL,
    days_before       INTEGER      NULL,
    audience          TEXT         NOT NULL DEFAULT 'office',
    scope             TEXT         NOT NULL DEFAULT 'all',
    -- scope = group: iletişim merkezinin mükellef grubu (#780 A); o gelene kadar serbest anahtar.
    group_ref         TEXT         NULL,
    -- scope = selected: seçili mükellef dosyaları (engagements.uuid).
    engagement_uuids  UUID[]       NOT NULL DEFAULT '{}',
    label             TEXT         NULL,
    enabled           BOOLEAN      NOT NULL DEFAULT TRUE,
    updated_by        BIGINT       NULL REFERENCES users (id) ON DELETE SET NULL,
    created_at        TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at        TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_vat_report_rules_uuid UNIQUE (uuid),
    CONSTRAINT chk_vat_report_rules_trigger CHECK (
        (trigger_kind = 'day' AND day BETWEEN 1 AND 31 AND days_before IS NULL)
        OR (trigger_kind = 'before_month_end' AND days_before BETWEEN 0 AND 27 AND day IS NULL)),
    CONSTRAINT chk_vat_report_rules_audience CHECK (audience IN ('office', 'taxpayer_approval', 'taxpayer_direct')),
    CONSTRAINT chk_vat_report_rules_scope CHECK (scope IN ('all', 'group', 'selected', 'payable_only'))
);

CREATE INDEX idx_vat_report_rules_org ON vat_report_rules (organization_id, id);
CREATE INDEX idx_vat_report_rules_updated_by ON vat_report_rules (updated_by);

SELECT rls_apply_org('vat_report_rules');

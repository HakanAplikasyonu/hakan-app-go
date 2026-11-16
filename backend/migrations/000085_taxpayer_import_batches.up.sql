-- Kurulum sihirbazı (issue #273): Excel / CSV'den toplu mükellef aktarımı.
-- Dosya yüklenince satırlar doğrulanır ve satır satır rapor bu tabloda
-- saklanır; sihirbaz yarıda bırakılıp sonra devam ettirilebilir. "Davet
-- gönder" adımı geçerli satırlar için mükellef ekleme talebi açar (onay
-- e-postası, #7) ve sonuçları outcome'a yazar. Ofisin iş verisi: ofis
-- kapsamlı, RLS'li.
CREATE TABLE taxpayer_import_batches (
    id               BIGSERIAL    PRIMARY KEY,
    uuid             UUID         NOT NULL DEFAULT gen_random_uuid(),
    organization_id  BIGINT       NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    file_name        TEXT         NOT NULL,
    -- Satırlar ve satır hataları (taxpayers.ImportRow).
    rows             JSONB        NOT NULL,
    total            INTEGER      NOT NULL,
    valid            INTEGER      NOT NULL,
    invalid          INTEGER      NOT NULL,
    created_by       BIGINT       NULL REFERENCES users (id) ON DELETE SET NULL,
    created_at       TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    -- Davetler gönderildiğinde: satır başına sonuç (taxpayers.ImportOutcome).
    sent_at          TIMESTAMPTZ  NULL,
    sent_by          BIGINT       NULL REFERENCES users (id) ON DELETE SET NULL,
    outcome          JSONB        NULL,
    CONSTRAINT uq_taxpayer_import_batches_uuid UNIQUE (uuid),
    CONSTRAINT chk_taxpayer_import_batches_counts CHECK (total >= 0 AND valid >= 0 AND invalid >= 0 AND valid + invalid = total)
);

CREATE INDEX idx_taxpayer_import_batches_org ON taxpayer_import_batches (organization_id, created_at DESC);

SELECT rls_apply_org('taxpayer_import_batches');

-- Sihirbazın durumu (adımlar, muhasebe programı, aktarım). Kendi sütununda:
-- PATCH /v1/tenant/settings settings nesnesini bütünüyle değiştirir, durumu
-- silmesin.
ALTER TABLE organizations ADD COLUMN setup_state JSONB NULL;

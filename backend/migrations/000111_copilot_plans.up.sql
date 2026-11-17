-- Co-pilot aksiyon planları (issue #392): müşavirin komut çubuğuna yazdığı
-- doğal dil komutundan (ör. "Atlas Gıda ekim fişlerini Luca'ya aktar")
-- çıkarılan, onay bekleyen adım listesi. Plan yalnızca öneridir: hiçbir adım
-- kullanıcı onaylamadan çalışmaz; onaylanan adımlar mevcut API uçlarıyla,
-- kullanıcının kendi yetkisiyle yürütülür ve sonucu buraya yazılır.
-- Ofis kapsamlı (organization_id); listeler created_by ile kullanıcıya
-- süzülür.
CREATE TABLE copilot_plans (
    id               BIGSERIAL    PRIMARY KEY,
    uuid             UUID         NOT NULL DEFAULT gen_random_uuid(),
    organization_id  BIGINT       NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    created_by       BIGINT       NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    command          TEXT         NOT NULL,
    period           CHAR(7)      NULL,
    -- proposed: onay bekliyor; running: adımlar yürütülüyor; done: bitti;
    -- cancelled: kullanıcı vazgeçti.
    status           TEXT         NOT NULL DEFAULT 'proposed',
    -- ai: dil modeli; rules: kural tabanlı çözümleyici (AI kapalıyken).
    planner          TEXT         NOT NULL,
    model            TEXT         NULL,
    -- [{action, engagement_uuid, taxpayer_title, period, status, error_code, finished_at}]
    steps            JSONB        NOT NULL DEFAULT '[]'::jsonb,
    -- Çözümleyicinin anlayamadığı kısımlar (kullanıcıya not olarak gösterilir).
    notes            JSONB        NOT NULL DEFAULT '[]'::jsonb,
    created_at       TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at       TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_copilot_plans_uuid UNIQUE (uuid),
    CONSTRAINT chk_copilot_plans_command CHECK (btrim(command) <> '' AND char_length(command) <= 500),
    CONSTRAINT chk_copilot_plans_period CHECK (period IS NULL OR period ~ '^[0-9]{4}-(0[1-9]|1[0-2])$'),
    CONSTRAINT chk_copilot_plans_status CHECK (status IN ('proposed', 'running', 'done', 'cancelled')),
    CONSTRAINT chk_copilot_plans_planner CHECK (planner IN ('ai', 'rules')),
    CONSTRAINT chk_copilot_plans_steps CHECK (jsonb_typeof(steps) = 'array' AND jsonb_array_length(steps) <= 50),
    CONSTRAINT chk_copilot_plans_notes CHECK (jsonb_typeof(notes) = 'array')
);

CREATE INDEX idx_copilot_plans_user ON copilot_plans (organization_id, created_by, created_at DESC);

CREATE TRIGGER trg_copilot_plans_set_updated_at
    BEFORE UPDATE ON copilot_plans FOR EACH ROW EXECUTE FUNCTION set_updated_at();

SELECT rls_apply_org('copilot_plans'::regclass);

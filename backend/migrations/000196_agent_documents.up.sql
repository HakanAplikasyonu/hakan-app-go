-- Yerel ajandan gelen dosyalar (#780 e: Luca'nın API'si yok). Müşavir
-- Luca'dan bordro dökümünü ve mizan Excel'ini, mükelleften gelen yabancı
-- hizmet faturası PDF'ini izlenen klasöre bırakır; masaüstü ajanı yükler.
-- Sunucu dosyayı içeriğinden tanır (bordro dökümü / mizan / hesap planı /
-- yabancı fatura), mükellefi dosyadaki VKN'den ya da klasör adından bulur
-- ve mevcut içe aktarmayı çalıştırır. Türü ya da mükellefi belli olmayan
-- dosya "eşleştir" kaydı olarak kalır; ofis mükellefi ve türü seçer.
--
-- Durum: needs_match (ofis eşleştirecek), imported (içe aktarıldı),
-- failed (içe aktarma reddetti; ofis yeniden eşleştirebilir), dismissed
-- (ofis yok saydı). Aynı içerik (sha256) ofiste bir kez kaydedilir.
CREATE TABLE agent_documents (
    id               BIGSERIAL    PRIMARY KEY,
    uuid             UUID         NOT NULL DEFAULT gen_random_uuid(),
    organization_id  BIGINT       NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    agent_id         BIGINT       NULL REFERENCES desktop_agents (id) ON DELETE SET NULL,
    engagement_id    BIGINT       NULL,
    folder           TEXT         NOT NULL DEFAULT '',
    file_name        TEXT         NOT NULL,
    sha256           TEXT         NOT NULL,
    size_bytes       BIGINT       NOT NULL,
    content_type     TEXT         NOT NULL DEFAULT 'application/octet-stream',
    storage_key      TEXT         NOT NULL,
    kind             TEXT         NOT NULL DEFAULT 'unknown',
    period           TEXT         NULL,
    match_source     TEXT         NOT NULL DEFAULT '',
    status           TEXT         NOT NULL DEFAULT 'needs_match',
    reason           TEXT         NOT NULL DEFAULT '',
    result_uuid      UUID         NULL,
    created_by       BIGINT       NULL REFERENCES users (id) ON DELETE SET NULL,
    resolved_by      BIGINT       NULL REFERENCES users (id) ON DELETE SET NULL,
    resolved_at      TIMESTAMPTZ  NULL,
    created_at       TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at       TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_agent_documents_uuid UNIQUE (uuid),
    CONSTRAINT uq_agent_documents_sha UNIQUE (organization_id, sha256),
    CONSTRAINT fk_agent_documents_engagement
        FOREIGN KEY (organization_id, engagement_id) REFERENCES engagements (organization_id, id) ON DELETE CASCADE,
    CONSTRAINT chk_agent_documents_kind
        CHECK (kind IN ('unknown', 'payroll', 'trial_balance', 'account_plan', 'foreign_invoice')),
    CONSTRAINT chk_agent_documents_status
        CHECK (status IN ('needs_match', 'imported', 'failed', 'dismissed')),
    CONSTRAINT chk_agent_documents_match_source
        CHECK (match_source IN ('', 'content', 'watch', 'folder', 'office')),
    CONSTRAINT chk_agent_documents_period CHECK (period IS NULL OR period ~ '^[0-9]{4}-(0[1-9]|1[0-2])$'),
    CONSTRAINT chk_agent_documents_file_name CHECK (char_length(file_name) BETWEEN 1 AND 255),
    CONSTRAINT chk_agent_documents_folder CHECK (char_length(folder) <= 1024),
    CONSTRAINT chk_agent_documents_reason CHECK (char_length(reason) <= 500)
);
-- Ofisin açık (eşleştirilecek) kayıtları, en yeni başta.
CREATE INDEX idx_agent_documents_org_status ON agent_documents (organization_id, status, id DESC);
CREATE INDEX idx_agent_documents_engagement ON agent_documents (organization_id, engagement_id);
CREATE INDEX idx_agent_documents_agent ON agent_documents (agent_id);
CREATE INDEX idx_agent_documents_created_by ON agent_documents (created_by);
CREATE INDEX idx_agent_documents_resolved_by ON agent_documents (resolved_by);
CREATE TRIGGER trg_agent_documents_set_updated_at
    BEFORE UPDATE ON agent_documents FOR EACH ROW EXECUTE FUNCTION set_updated_at();
SELECT rls_apply_org('agent_documents');

-- Ajandan gelen yabancı fatura PDF'i KDV2 taslağı olarak açılır.
ALTER TABLE foreign_service_invoices DROP CONSTRAINT chk_foreign_service_invoices_source_kind;
ALTER TABLE foreign_service_invoices ADD CONSTRAINT chk_foreign_service_invoices_source_kind
    CHECK (source_kind IS NULL OR source_kind IN ('taxpayer_upload', 'whatsapp', 'email', 'agent'));

-- Hesap kodu kural motoru ve kural hafızası (issue #25, docs/00 K6 T1,
-- docs/01 §3 "Procedures" ve "Carries context forward").
--
-- account_rules: ofis şablonu (engagement_id NULL) ve mükellef (engagement)
-- kuralları. Her kural sürümlüdür: rule_key kuralın kalıcı kimliği, her
-- değişiklik yeni bir (rule_key, version) satırıdır; önceki sürüm 'retired'
-- olur ve içeriği değiştirilemez (trg_account_rules_immutable). Öneriler ve
-- atamalar kuralın sürüm satırına (id) bağlanır; kural değişse de geçmiş
-- kayıtlar değişmez.
--
-- Çözüm sırası: mükellef kuralları > platform varsayılanları (kodda). Ofis
-- şablonu yeni mükellefe engagement açılırken kopyalanır
-- (trg_engagements_copy_account_rules).
CREATE TABLE account_rules (
    id              BIGSERIAL     PRIMARY KEY,
    uuid            UUID          NOT NULL DEFAULT gen_random_uuid(),
    organization_id BIGINT        NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    engagement_id   BIGINT        NULL,
    rule_key        UUID          NOT NULL DEFAULT gen_random_uuid(),
    version         INT           NOT NULL DEFAULT 1,
    status          TEXT          NOT NULL DEFAULT 'active',
    role            TEXT          NOT NULL DEFAULT 'line',
    priority        INT           NOT NULL DEFAULT 100,
    name            TEXT          NOT NULL DEFAULT '',
    conditions      JSONB         NOT NULL DEFAULT '{}'::jsonb,
    account_code    VARCHAR(64)   NOT NULL,
    confidence      NUMERIC(5,4)  NOT NULL DEFAULT 0.9000,
    origin          TEXT          NOT NULL DEFAULT 'manual',
    copied_from     BIGINT        NULL REFERENCES account_rules (id) ON DELETE SET NULL,
    created_by      BIGINT        NULL REFERENCES users (id) ON DELETE SET NULL,
    created_at      TIMESTAMPTZ   NOT NULL DEFAULT NOW(),
    retired_at      TIMESTAMPTZ   NULL,
    CONSTRAINT uq_account_rules_uuid UNIQUE (uuid),
    CONSTRAINT uq_account_rules_org_id UNIQUE (organization_id, id),
    CONSTRAINT uq_account_rules_version UNIQUE (rule_key, version),
    CONSTRAINT fk_account_rules_engagement
        FOREIGN KEY (organization_id, engagement_id) REFERENCES engagements (organization_id, id) ON DELETE CASCADE,
    CONSTRAINT chk_account_rules_status CHECK (status IN ('active', 'retired')),
    CONSTRAINT chk_account_rules_retired CHECK ((status = 'retired') = (retired_at IS NOT NULL)),
    CONSTRAINT chk_account_rules_role CHECK (role IN ('line', 'vat', 'counterparty')),
    CONSTRAINT chk_account_rules_conditions CHECK (jsonb_typeof(conditions) = 'object'),
    CONSTRAINT chk_account_rules_code CHECK (account_code ~ '^[0-9]{3}(\.[0-9A-Za-z]+)*$'),
    CONSTRAINT chk_account_rules_confidence CHECK (confidence >= 0 AND confidence <= 1),
    CONSTRAINT chk_account_rules_origin CHECK (origin IN ('manual', 'candidate', 'template')),
    CONSTRAINT chk_account_rules_version CHECK (version >= 1)
);

-- Bir kuralın tek etkin sürümü.
CREATE UNIQUE INDEX uq_account_rules_active ON account_rules (rule_key) WHERE status = 'active';
CREATE INDEX idx_account_rules_scope ON account_rules (organization_id, engagement_id, status, role, priority);

-- Sürüm satırı değiştirilemez: yalnızca active -> retired geçişi yazılabilir.
CREATE OR REPLACE FUNCTION account_rules_immutable() RETURNS trigger AS $$
BEGIN
    IF NEW.conditions IS DISTINCT FROM OLD.conditions
       OR NEW.account_code IS DISTINCT FROM OLD.account_code
       OR NEW.role IS DISTINCT FROM OLD.role
       OR NEW.priority IS DISTINCT FROM OLD.priority
       OR NEW.confidence IS DISTINCT FROM OLD.confidence
       OR NEW.version IS DISTINCT FROM OLD.version
       OR NEW.rule_key IS DISTINCT FROM OLD.rule_key
       OR NEW.engagement_id IS DISTINCT FROM OLD.engagement_id
       OR NEW.organization_id IS DISTINCT FROM OLD.organization_id
       OR (OLD.status = 'retired' AND NEW.status <> 'retired') THEN
        RAISE EXCEPTION 'account_rules: a rule version is immutable; create a new version';
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_account_rules_immutable
    BEFORE UPDATE ON account_rules FOR EACH ROW EXECUTE FUNCTION account_rules_immutable();

-- Yeni mükellef ofis şablonundan kural setini alır (docs/01 §3 Procedures).
CREATE OR REPLACE FUNCTION copy_office_account_rules(p_organization_id BIGINT, p_engagement_id BIGINT)
RETURNS INT AS $$
DECLARE n INT;
BEGIN
    INSERT INTO account_rules (organization_id, engagement_id, role, priority, name, conditions,
                               account_code, confidence, origin, copied_from, created_by)
    SELECT t.organization_id, p_engagement_id, t.role, t.priority, t.name, t.conditions,
           t.account_code, t.confidence, 'template', t.id, t.created_by
    FROM account_rules t
    WHERE t.organization_id = p_organization_id AND t.engagement_id IS NULL AND t.status = 'active'
      AND NOT EXISTS (
          SELECT 1 FROM account_rules e
          JOIN account_rules src ON src.id = e.copied_from
          WHERE e.organization_id = p_organization_id AND e.engagement_id = p_engagement_id
            AND src.rule_key = t.rule_key);
    GET DIAGNOSTICS n = ROW_COUNT;
    RETURN n;
END;
$$ LANGUAGE plpgsql;

CREATE OR REPLACE FUNCTION engagements_copy_account_rules() RETURNS trigger AS $$
BEGIN
    PERFORM copy_office_account_rules(NEW.organization_id, NEW.id);
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_engagements_copy_account_rules
    AFTER INSERT ON engagements FOR EACH ROW EXECUTE FUNCTION engagements_copy_account_rules();

-- account_assignments: uygulanmış hesap kodu (ofis iş ürünü). Kaynak veri
-- (invoice_items) değişmez; hesap kodu ofisin kararıdır (D2). İnceleme
-- kuyruğu (#14) "account_code" önerisini kabul edince buraya yazar.
CREATE TABLE account_assignments (
    id              BIGSERIAL    PRIMARY KEY,
    organization_id BIGINT       NOT NULL,
    engagement_id   BIGINT       NOT NULL,
    taxpayer_id     BIGINT       NOT NULL REFERENCES taxpayers (id) ON DELETE RESTRICT,
    target_type     TEXT         NOT NULL,
    target_id       BIGINT       NOT NULL,
    field           TEXT         NOT NULL,
    account_code    VARCHAR(64)  NOT NULL,
    rule_id         BIGINT       NULL REFERENCES account_rules (id) ON DELETE SET NULL,
    suggestion_id   BIGINT       NULL,
    assigned_by     BIGINT       NULL REFERENCES users (id) ON DELETE SET NULL,
    created_at      TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_account_assignments_target UNIQUE (engagement_id, target_type, target_id, field),
    CONSTRAINT fk_account_assignments_engagement
        FOREIGN KEY (organization_id, engagement_id) REFERENCES engagements (organization_id, id) ON DELETE CASCADE,
    CONSTRAINT chk_account_assignments_target CHECK (target_type IN ('invoice_item', 'customer')),
    CONSTRAINT chk_account_assignments_field CHECK (field IN ('account_code', 'vat_account_code')),
    CONSTRAINT chk_account_assignments_code CHECK (account_code ~ '^[0-9]{3}(\.[0-9A-Za-z]+)*$')
);

CREATE TRIGGER trg_account_assignments_set_updated_at
    BEFORE UPDATE ON account_assignments FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- Kural adayı sorgusu: aynı karşı taraf için onaylanan düzeltmeler.
CREATE INDEX idx_suggestions_account_memory
    ON suggestions (organization_id, engagement_id, kind, status)
    WHERE kind IN ('account_code', 'counterparty_account');

-- RLS (issue #6).
SELECT rls_apply_org('account_rules'::regclass);
SELECT rls_apply_engagement('account_assignments'::regclass);

INSERT INTO permissions (name, slug) VALUES
    ('Read tenant account rules', 'tenant.account_rules.read'),
    ('Write tenant account rules', 'tenant.account_rules.write')
ON CONFLICT (slug) DO NOTHING;

SELECT grant_tenant_role_permission('admin', 'tenant.account_rules.read');
SELECT grant_tenant_role_permission('admin', 'tenant.account_rules.write');
SELECT grant_tenant_role_permission('accountant', 'tenant.account_rules.read');
SELECT grant_tenant_role_permission('accountant', 'tenant.account_rules.write');
SELECT grant_tenant_role_permission('member', 'tenant.account_rules.read');
SELECT grant_tenant_role_permission('intern', 'tenant.account_rules.read');

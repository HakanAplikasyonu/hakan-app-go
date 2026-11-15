-- Hedef hesap planı (issue #30, docs/02 §2): ofisin Luca'daki hesap planı ya
-- da mizan dökümü mükellef (engagement) bazında içe alınır. Çıktı öncesi
-- yevmiye satırlarındaki hesap kodları bu plana karşı denetlenir; planda
-- olmayan kod ACCOUNT_NOT_IN_TARGET_PLAN üretir (Luca'nın "hesap kodu
-- bulunamadı" hatası çıktıdan önce yakalanır).
--
-- Ofis iş ürünü: organization_id + engagement_id (D2: ofis değişince eski
-- ofiste kalır). Her içe aktarma bir sürümdür; account_plan_accounts yalnızca
-- etkin (son) sürümün satırlarını tutar, eski sürümün özeti (dosya, SHA-256,
-- satır sayısı) account_plan_imports'ta kalır.
CREATE TABLE account_plan_imports (
    id              BIGSERIAL    PRIMARY KEY,
    uuid            UUID         NOT NULL DEFAULT gen_random_uuid(),
    organization_id BIGINT       NOT NULL,
    engagement_id   BIGINT       NOT NULL,
    target          TEXT         NOT NULL DEFAULT 'luca',
    source_format   TEXT         NOT NULL,
    source_kind     TEXT         NOT NULL,
    file_name       TEXT         NOT NULL DEFAULT '',
    sha256          CHAR(64)     NOT NULL,
    account_count   INT          NOT NULL DEFAULT 0,
    skipped_rows    INT          NOT NULL DEFAULT 0,
    imported_by     BIGINT       NULL REFERENCES users (id) ON DELETE SET NULL,
    created_at      TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_account_plan_imports_uuid UNIQUE (uuid),
    CONSTRAINT uq_account_plan_imports_org_id UNIQUE (organization_id, id),
    CONSTRAINT fk_account_plan_imports_engagement
        FOREIGN KEY (organization_id, engagement_id) REFERENCES engagements (organization_id, id) ON DELETE CASCADE,
    CONSTRAINT chk_account_plan_imports_target CHECK (target ~ '^[a-z][a-z0-9_]*$'),
    CONSTRAINT chk_account_plan_imports_format CHECK (source_format IN ('xlsx', 'csv')),
    CONSTRAINT chk_account_plan_imports_kind CHECK (source_kind IN ('account_plan', 'trial_balance'))
);

CREATE INDEX idx_account_plan_imports_engagement ON account_plan_imports (organization_id, engagement_id, id DESC);

CREATE TABLE account_plan_accounts (
    id              BIGSERIAL    PRIMARY KEY,
    organization_id BIGINT       NOT NULL,
    engagement_id   BIGINT       NOT NULL,
    import_id       BIGINT       NOT NULL,
    target          TEXT         NOT NULL DEFAULT 'luca',
    -- code: dosyadaki yazım (boşluk / tire noktaya çevrilmiş);
    -- denetim bu alanla birebir yapılır.
    code            VARCHAR(64)  NOT NULL,
    name            TEXT         NOT NULL DEFAULT '',
    main_code       CHAR(3)      NOT NULL,
    level           SMALLINT     NOT NULL,
    -- is_leaf: altında başka kod yok (fişe yazılabilir hesap).
    is_leaf         BOOLEAN      NOT NULL DEFAULT true,
    CONSTRAINT uq_account_plan_accounts_code UNIQUE (engagement_id, target, code),
    CONSTRAINT fk_account_plan_accounts_import
        FOREIGN KEY (organization_id, import_id) REFERENCES account_plan_imports (organization_id, id) ON DELETE CASCADE,
    CONSTRAINT fk_account_plan_accounts_engagement
        FOREIGN KEY (organization_id, engagement_id) REFERENCES engagements (organization_id, id) ON DELETE CASCADE,
    CONSTRAINT chk_account_plan_accounts_code CHECK (code ~ '^[0-9]{3}(\.[0-9A-Za-z]+)*$'),
    CONSTRAINT chk_account_plan_accounts_level CHECK (level >= 1)
);

CREATE INDEX idx_account_plan_accounts_list ON account_plan_accounts (organization_id, engagement_id, target, code);

-- RLS (issue #6): engagement kapsamlı iki tablo.
SELECT rls_apply_engagement(t) FROM unnest(ARRAY['account_plan_imports', 'account_plan_accounts']::regclass[]) AS t;

INSERT INTO permissions (name, slug) VALUES
    ('Read tenant account plans', 'tenant.account_plan.read'),
    ('Write tenant account plans', 'tenant.account_plan.write')
ON CONFLICT (slug) DO NOTHING;

SELECT grant_tenant_role_permission('admin', 'tenant.account_plan.read');
SELECT grant_tenant_role_permission('admin', 'tenant.account_plan.write');
SELECT grant_tenant_role_permission('accountant', 'tenant.account_plan.read');
SELECT grant_tenant_role_permission('accountant', 'tenant.account_plan.write');
SELECT grant_tenant_role_permission('member', 'tenant.account_plan.read');
SELECT grant_tenant_role_permission('intern', 'tenant.account_plan.read');

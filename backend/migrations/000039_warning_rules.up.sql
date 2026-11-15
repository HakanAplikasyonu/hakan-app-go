-- Uyarı motoru yapılandırması (issue #13, 05-uyari-kodlari.md).
--
-- warning_codes: platform kataloğu. Her kodun ÖNERİLEN seviyesi ve varsayılan
-- eşikleri burada; kodda sabit değil (05 §2). Kod listesi
-- internal/modules/warnings/catalog ile aynı tutulur (catalog_test.go).
CREATE TABLE warning_codes (
    code             VARCHAR(64) PRIMARY KEY,
    section          VARCHAR(8)  NOT NULL,
    default_severity TEXT        NOT NULL,
    default_params   JSONB       NOT NULL DEFAULT '{}'::jsonb,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT chk_warning_codes_severity CHECK (default_severity IN ('info', 'warning', 'blocker')),
    CONSTRAINT chk_warning_codes_params CHECK (jsonb_typeof(default_params) = 'object')
);

CREATE TRIGGER trg_warning_codes_set_updated_at
    BEFORE UPDATE ON warning_codes
    FOR EACH ROW
    EXECUTE FUNCTION set_updated_at();

-- warning_rule_configs: ofis (engagement_id NULL) ve mükellef (engagement)
-- bazında seviye, açık/kapalı ve eşik ayarı. Çözüm sırası:
-- engagement > ofis > warning_codes varsayılanı. NULL severity "üst
-- seviyeden al" demektir; params üst seviyenin üstüne anahtar bazında yazılır.
CREATE TABLE warning_rule_configs (
    id              BIGSERIAL PRIMARY KEY,
    organization_id BIGINT      NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    engagement_id   BIGINT      NULL,
    code            VARCHAR(64) NOT NULL REFERENCES warning_codes (code) ON DELETE CASCADE,
    severity        TEXT        NULL,
    enabled         BOOLEAN     NULL,
    params          JSONB       NOT NULL DEFAULT '{}'::jsonb,
    updated_by      BIGINT      NULL REFERENCES users (id) ON DELETE SET NULL,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT fk_warning_rule_configs_engagement
        FOREIGN KEY (organization_id, engagement_id)
        REFERENCES engagements (organization_id, id) ON DELETE CASCADE,
    CONSTRAINT chk_warning_rule_configs_severity CHECK (severity IS NULL OR severity IN ('info', 'warning', 'blocker')),
    CONSTRAINT chk_warning_rule_configs_params CHECK (jsonb_typeof(params) = 'object')
);

CREATE UNIQUE INDEX uq_warning_rule_configs_office
    ON warning_rule_configs (organization_id, code)
    WHERE engagement_id IS NULL;
CREATE UNIQUE INDEX uq_warning_rule_configs_engagement
    ON warning_rule_configs (organization_id, engagement_id, code)
    WHERE engagement_id IS NOT NULL;

CREATE TRIGGER trg_warning_rule_configs_set_updated_at
    BEFORE UPDATE ON warning_rule_configs
    FOR EACH ROW
    EXECUTE FUNCTION set_updated_at();

-- Katalog. Seviyeler 05'teki önerilerdir. Eşikler (params) ürün kararıdır;
-- NULL eşik "ofis girmeden kural çalışmaz" demektir.
INSERT INTO warning_codes (code, section, default_severity, default_params) VALUES
    -- §3.1 Fatura ve UBL
    ('PENDING_RESPONSE',        '3.1', 'info',    '{}'),
    ('REJECTED',                '3.1', 'warning', '{}'),
    ('RETURNED',                '3.1', 'warning', '{}'),
    ('DELIVERY_ERROR',          '3.1', 'blocker', '{}'),
    ('STATUS_UNMAPPED',         '3.1', 'warning', '{}'),
    ('STATUS_UNVERIFIED',       '3.1', 'info',    '{}'),
    ('EARCHIVE_REPORT_ERROR',   '3.1', 'warning', '{}'),
    ('DOC_KIND_MISMATCH',       '3.1', 'warning', '{}'),
    ('UUID_MISMATCH',           '3.1', 'blocker', '{}'),
    ('PARTY_MISMATCH',          '3.1', 'blocker', '{}'),
    ('UNMAPPED_TYPE_CODE',      '3.1', 'warning', '{}'),
    ('UNMAPPED_PROFILE',        '3.1', 'info',    '{}'),
    ('UNKNOWN_TAX_CODE',        '3.1', 'warning', '{}'),
    ('LINE_TAXABLE_MISMATCH',   '3.1', 'warning', '{"tolerance": "0.01"}'),
    ('TAX_TOTAL_MISMATCH',      '3.1', 'warning', '{"tolerance": "0.05"}'),
    ('BASE_TOTAL_MISMATCH',     '3.1', 'warning', '{"tolerance": "0.05"}'),
    ('PAYABLE_MISMATCH',        '3.1', 'warning', '{"tolerance": "0.05"}'),
    ('HEADER_ESTIMATE',         '3.1', 'warning', '{}'),
    ('UBL_MISSING',             '3.1', 'warning', '{}'),
    ('MISSING_AT_SOURCE',       '3.1', 'warning', '{}'),
    ('UNPROCESSED_INBOX',       '3.1', 'info',    '{}'),
    -- §3.2 Genel veri kalitesi
    ('POSSIBLE_DUPLICATE',      '3.2', 'warning', '{}'),
    ('INVALID_TAX_ID',          '3.2', 'warning', '{}'),
    ('DELETED_AT_SOURCE',       '3.2', 'warning', '{}'),
    ('LOCKED_PERIOD_CHANGE',    '3.2', 'blocker', '{}'),
    ('VAT_REPORT_MISMATCH',     '3.2', 'warning', '{"tolerance": "0.05"}'),
    ('OUT_OF_PERIOD',           '3.2', 'warning', '{}'),
    ('UNAPPROVED_EXPENSE',      '3.2', 'info',    '{}'),
    ('RETAIL_ALREADY_INVOICED', '3.2', 'info',    '{}'),
    -- §3.3 Müşavir talebinden gelen kontroller
    ('FIXED_ASSET_CANDIDATE',   '3.3', 'warning', '{}'),
    ('UNUSUAL_RECORD_TYPE',     '3.3', 'warning', '{}'),
    ('UNUSUAL_AMOUNT',          '3.3', 'warning', '{"multiplier": "3", "min_history": 3}'),
    ('NEW_PARTY_HIGH_AMOUNT',   '3.3', 'warning', '{"limit": "50000"}'),
    ('PLATE_NOT_REGISTERED',    '3.3', 'warning', '{}'),
    -- §3.4 Banka (21 §6.3): BNK_GAP ve BNK_BALANCE_MISMATCH blocker, diğerleri warning
    ('BNK_UNINVOICED_COLLECTION',   '3.4', 'warning', '{}'),
    ('BNK_UNDOCUMENTED_PAYMENT',    '3.4', 'warning', '{}'),
    ('BNK_FEES_NOT_BOOKED',         '3.4', 'warning', '{}'),
    ('BNK_INTEREST',                '3.4', 'warning', '{}'),
    ('BNK_TAX_PAID',                '3.4', 'warning', '{}'),
    ('BNK_RENT_WITHHOLDING',        '3.4', 'warning', '{}'),
    ('BNK_SALARY_VS_PAYROLL',       '3.4', 'warning', '{}'),
    ('BNK_PAYMENT_METHOD_MISMATCH', '3.4', 'warning', '{}'),
    ('BNK_CASH_LIMIT',              '3.4', 'warning', '{"limit": null}'),
    ('BNK_POS_VS_SALES',            '3.4', 'warning', '{"tolerance_days": 3}'),
    ('BNK_BALANCE_MISMATCH',        '3.4', 'blocker', '{}'),
    ('BNK_GAP',                     '3.4', 'blocker', '{}'),
    ('BNK_NON_BUSINESS',            '3.4', 'warning', '{}'),
    ('BNK_UNCLASSIFIED',            '3.4', 'warning', '{}'),
    -- HAKAN eklemeleri
    ('UBL_SCHEMATRON_FAIL',         '01',  'blocker', '{}'),
    ('ACCOUNT_NOT_IN_TARGET_PLAN',  '02',  'blocker', '{}');

INSERT INTO permissions (name, slug) VALUES
    ('Read tenant warnings', 'tenant.warnings.read'),
    ('Write tenant warnings', 'tenant.warnings.write'),
    ('Write tenant warning rules', 'tenant.warning_rules.write')
ON CONFLICT (slug) DO NOTHING;

-- owner/admin: hepsi. member: okuma ve uyarıyı kabul/çözme (kural ayarı değil).
INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM roles r
JOIN permissions p ON p.slug IN ('tenant.warnings.read', 'tenant.warnings.write', 'tenant.warning_rules.write')
WHERE r.slug IN ('owner', 'admin') AND r.scope = 'tenant' AND r.organization_id IS NULL
ON CONFLICT DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM roles r
JOIN permissions p ON p.slug IN ('tenant.warnings.read', 'tenant.warnings.write')
WHERE r.slug = 'member' AND r.scope = 'tenant' AND r.organization_id IS NULL
ON CONFLICT DO NOTHING;

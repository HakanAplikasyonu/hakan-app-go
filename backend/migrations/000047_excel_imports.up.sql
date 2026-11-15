-- Excel içe aktarma (issue #22, 04-excel-ice-aktarma.md).
CREATE TABLE excel_import_templates (
    id              BIGSERIAL    PRIMARY KEY,
    uuid            UUID         NOT NULL DEFAULT gen_random_uuid(),
    -- company: bu ofiste bu mükellef; tenant: ofisin bütün mükellefleri;
    -- global: platformun hazır şablonu (04 §6).
    scope           TEXT         NOT NULL,
    organization_id BIGINT       NULL REFERENCES organizations (id) ON DELETE CASCADE,
    taxpayer_id     BIGINT       NULL REFERENCES taxpayers (id) ON DELETE CASCADE,
    fingerprint     CHAR(64)     NOT NULL,
    data_type       TEXT         NOT NULL,
    source_system   TEXT         NOT NULL,
    header_row      INT          NOT NULL,
    columns         JSONB        NOT NULL,
    created_by      BIGINT       NULL REFERENCES users (id) ON DELETE SET NULL,
    created_at      TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_excel_import_templates_uuid UNIQUE (uuid),
    CONSTRAINT chk_excel_import_templates_scope CHECK (
        (scope = 'company' AND organization_id IS NOT NULL AND taxpayer_id IS NOT NULL)
        OR (scope = 'tenant' AND organization_id IS NOT NULL AND taxpayer_id IS NULL)
        OR (scope = 'global' AND organization_id IS NULL AND taxpayer_id IS NULL)),
    CONSTRAINT chk_excel_import_templates_fingerprint CHECK (fingerprint ~ '^[0-9a-f]{64}$'),
    CONSTRAINT chk_excel_import_templates_columns CHECK (jsonb_typeof(columns) = 'array')
);

CREATE UNIQUE INDEX uq_excel_import_templates_key ON excel_import_templates
    (scope, COALESCE(organization_id, 0), COALESCE(taxpayer_id, 0), fingerprint, data_type);
CREATE INDEX idx_excel_import_templates_fp ON excel_import_templates (fingerprint);

CREATE TRIGGER trg_excel_import_templates_set_updated_at
    BEFORE UPDATE ON excel_import_templates FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE TABLE excel_imports (
    id              BIGSERIAL    PRIMARY KEY,
    uuid            UUID         NOT NULL DEFAULT gen_random_uuid(),
    organization_id BIGINT       NOT NULL,
    engagement_id   BIGINT       NOT NULL,
    taxpayer_id     BIGINT       NOT NULL REFERENCES taxpayers (id) ON DELETE RESTRICT,
    created_by      BIGINT       NULL REFERENCES users (id) ON DELETE SET NULL,
    file_name       TEXT         NOT NULL,
    format          TEXT         NOT NULL,
    sha256          CHAR(64)     NOT NULL,
    size_bytes      BIGINT       NOT NULL,
    storage_key     TEXT         NOT NULL,
    attachment_id   BIGINT       NULL,
    data_type       TEXT         NULL,
    source_system   TEXT         NULL,
    period          CHAR(7)      NULL,
    sheet_name      TEXT         NULL,
    layout          JSONB        NOT NULL DEFAULT '{}'::jsonb,
    mapping         JSONB        NOT NULL DEFAULT '{}'::jsonb,
    template_id     BIGINT       NULL REFERENCES excel_import_templates (id) ON DELETE SET NULL,
    ai_used         BOOLEAN      NOT NULL DEFAULT false,
    status          TEXT         NOT NULL DEFAULT 'uploaded',
    summary         JSONB        NOT NULL DEFAULT '{}'::jsonb,
    error           TEXT         NULL,
    idempotency_key VARCHAR(128) NULL,
    committed_by    BIGINT       NULL REFERENCES users (id) ON DELETE SET NULL,
    committed_at    TIMESTAMPTZ  NULL,
    rolled_back_by  BIGINT       NULL REFERENCES users (id) ON DELETE SET NULL,
    rolled_back_at  TIMESTAMPTZ  NULL,
    created_at      TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_excel_imports_uuid UNIQUE (uuid),
    CONSTRAINT uq_excel_imports_org_id UNIQUE (organization_id, id),
    CONSTRAINT fk_excel_imports_engagement
        FOREIGN KEY (organization_id, engagement_id) REFERENCES engagements (organization_id, id) ON DELETE CASCADE,
    CONSTRAINT fk_excel_imports_attachment
        FOREIGN KEY (taxpayer_id, attachment_id) REFERENCES attachments (taxpayer_id, id),
    CONSTRAINT chk_excel_imports_format CHECK (format IN ('xlsx', 'csv')),
    CONSTRAINT chk_excel_imports_status CHECK (status IN (
        'uploaded', 'analyzing', 'mapping_review', 'validating', 'has_errors', 'preview',
        'committed', 'failed', 'rolled_back')),
    CONSTRAINT chk_excel_imports_period CHECK (period IS NULL OR period ~ '^[0-9]{4}-(0[1-9]|1[0-2])$')
);

CREATE INDEX idx_excel_imports_list ON excel_imports (organization_id, engagement_id, created_at DESC);

CREATE TRIGGER trg_excel_imports_set_updated_at
    BEFORE UPDATE ON excel_imports FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE TABLE excel_import_rows (
    id              BIGSERIAL    PRIMARY KEY,
    organization_id BIGINT       NOT NULL,
    engagement_id   BIGINT       NOT NULL,
    import_id       BIGINT       NOT NULL,
    row_no          INT          NOT NULL,
    raw             JSONB        NOT NULL,
    record          JSONB        NULL,
    result          TEXT         NOT NULL,
    messages        JSONB        NOT NULL DEFAULT '[]'::jsonb,
    skipped         BOOLEAN      NOT NULL DEFAULT false,
    -- Yazma sonucu: oluşturulan fatura veya zaten var olan (mükerrer).
    invoice_id      BIGINT       NULL,
    duplicate       BOOLEAN      NOT NULL DEFAULT false,
    CONSTRAINT uq_excel_import_rows UNIQUE (import_id, row_no),
    CONSTRAINT fk_excel_import_rows_import
        FOREIGN KEY (organization_id, import_id) REFERENCES excel_imports (organization_id, id) ON DELETE CASCADE,
    CONSTRAINT chk_excel_import_rows_result CHECK (result IN ('ok', 'warning', 'error', 'skipped'))
);

-- RLS (issue #6): engagement kapsamlı iki tablo.
SELECT rls_apply_engagement(t) FROM unnest(ARRAY['excel_imports', 'excel_import_rows']::regclass[]) AS t;

INSERT INTO permissions (name, slug) VALUES
    ('Read tenant Excel imports', 'tenant.excel_imports.read'),
    ('Write tenant Excel imports', 'tenant.excel_imports.write'),
    ('Commit tenant Excel imports', 'tenant.excel_imports.commit')
ON CONFLICT (slug) DO NOTHING;

-- owner/admin: hepsi. member: yükler, eşler, doğrular; commit ve rollback müşavir yetkisi.
INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM roles r
JOIN permissions p ON p.slug IN ('tenant.excel_imports.read', 'tenant.excel_imports.write', 'tenant.excel_imports.commit')
WHERE r.slug IN ('owner', 'admin') AND r.scope = 'tenant' AND r.organization_id IS NULL
ON CONFLICT DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM roles r
JOIN permissions p ON p.slug IN ('tenant.excel_imports.read', 'tenant.excel_imports.write')
WHERE r.slug = 'member' AND r.scope = 'tenant' AND r.organization_id IS NULL
ON CONFLICT DO NOTHING;

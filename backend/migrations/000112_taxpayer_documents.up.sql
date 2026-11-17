-- Belge arşivi ve İSG evrakları (#393).
--
-- Ofisin mükellef için sakladığı geçmiş evraklar (sözleşme, vergi levhası,
-- beyanname, SGK, ticaret sicil...) ve İSG evrakları (İSG eğitim sertifikası,
-- risk değerlendirmesi, sağlık raporu). Evrak mükellefe aittir:
-- payroll_documents gibi taxpayer_id kapsamlı (rls_apply_taxpayer); mükellef
-- başka ofise geçse de evrak mükellefte kalır, ofis yalnızca görünür
-- engagement'larının mükelleflerini görür.
--
-- İSG evrakının geçerlilik bitişi (valid_until) yaklaşan bitiş uyarısını
-- besler (ofis mobil uygulaması, Belgeler → İSG).

-- roles ve role_permissions FORCE altında (000086): izin kopyaları ofis
-- rollerine sistem kipinde yazılır.
SET LOCAL app.system = 'on';

INSERT INTO permissions (name, slug) VALUES
    ('Read taxpayer document archive', 'tenant.documents.read'),
    ('Write taxpayer document archive', 'tenant.documents.write')
ON CONFLICT (slug) DO NOTHING;

SELECT grant_tenant_role_permission(r, 'tenant.documents.read')
FROM unnest(ARRAY['admin', 'accountant', 'member', 'intern']) AS r;
SELECT grant_tenant_role_permission(r, 'tenant.documents.write')
FROM unnest(ARRAY['admin', 'accountant', 'member']) AS r;

CREATE TABLE taxpayer_documents (
    id                   BIGSERIAL    PRIMARY KEY,
    uuid                 UUID         NOT NULL DEFAULT gen_random_uuid(),
    taxpayer_id          BIGINT       NOT NULL REFERENCES taxpayers (id) ON DELETE RESTRICT,
    -- Belge türü kategorisi. isg: İSG evrakı (isg_kind zorunlu).
    category             TEXT         NOT NULL DEFAULT 'general',
    isg_kind             TEXT         NULL,
    title                TEXT         NOT NULL,
    note                 TEXT         NOT NULL DEFAULT '',
    -- Belgenin tarihi (düzenlenme) ve geçerlilik bitişi (İSG, sözleşme...).
    document_date        DATE         NULL,
    valid_until          DATE         NULL,
    storage_key          TEXT         NOT NULL,
    sha256               CHAR(64)     NOT NULL,
    size_bytes           BIGINT       NOT NULL,
    content_type         TEXT         NOT NULL,
    file_name            TEXT         NOT NULL,
    visible_to_taxpayer  BOOLEAN      NOT NULL DEFAULT FALSE,
    created_by           BIGINT       NULL REFERENCES users (id) ON DELETE SET NULL,
    created_at           TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at           TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_taxpayer_documents_uuid UNIQUE (uuid),
    CONSTRAINT uq_taxpayer_documents_sha256 UNIQUE (taxpayer_id, sha256),
    CONSTRAINT chk_taxpayer_documents_category
        CHECK (category IN ('general', 'contract', 'tax', 'sgk', 'trade_registry', 'isg', 'other')),
    CONSTRAINT chk_taxpayer_documents_isg_kind
        CHECK ((category = 'isg') = (isg_kind IS NOT NULL)
               AND (isg_kind IS NULL OR isg_kind IN ('isg_training', 'risk_assessment', 'health_report', 'other'))),
    CONSTRAINT chk_taxpayer_documents_title CHECK (btrim(title) <> '' AND char_length(title) <= 200),
    CONSTRAINT chk_taxpayer_documents_note CHECK (char_length(note) <= 1000),
    CONSTRAINT chk_taxpayer_documents_validity CHECK (valid_until IS NULL OR document_date IS NULL OR valid_until >= document_date),
    CONSTRAINT chk_taxpayer_documents_sha256 CHECK (sha256 ~ '^[0-9a-f]{64}$'),
    CONSTRAINT chk_taxpayer_documents_size CHECK (size_bytes > 0)
);

CREATE INDEX idx_taxpayer_documents_list ON taxpayer_documents (taxpayer_id, category, created_at DESC);
CREATE INDEX idx_taxpayer_documents_expiry ON taxpayer_documents (taxpayer_id, valid_until) WHERE valid_until IS NOT NULL;

CREATE TRIGGER trg_taxpayer_documents_set_updated_at
    BEFORE UPDATE ON taxpayer_documents FOR EACH ROW EXECUTE FUNCTION set_updated_at();

SELECT rls_apply_taxpayer('taxpayer_documents'::regclass);

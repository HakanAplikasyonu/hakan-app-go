-- Tenant-core faz 6a (issue #2): ofis içi alt kapsam.
--
-- taxpayers: platform düzeyinde mükellef kimliği. Bu migration yalnızca
-- engagement'ın bağlanacağı en sade tabloyu açar; VKN/TCKN, unvan ve vergi
-- dairesi alanları faz 6b'de (issue #3) eklenir.
CREATE TABLE taxpayers (
    id         BIGSERIAL PRIMARY KEY,
    uuid       UUID        NOT NULL DEFAULT gen_random_uuid(),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_taxpayers_uuid UNIQUE (uuid)
);

CREATE TRIGGER trg_taxpayers_set_updated_at
    BEFORE UPDATE ON taxpayers
    FOR EACH ROW
    EXECUTE FUNCTION set_updated_at();

-- engagements: ofis <-> mükellef hizmet ilişkisi. Bitince ended_at dolar.
CREATE TABLE engagements (
    id              BIGSERIAL PRIMARY KEY,
    uuid            UUID        NOT NULL DEFAULT gen_random_uuid(),
    organization_id BIGINT      NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    taxpayer_id     BIGINT      NOT NULL REFERENCES taxpayers (id) ON DELETE RESTRICT,
    status          TEXT        NOT NULL DEFAULT 'active',
    started_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    ended_at        TIMESTAMPTZ NULL,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_engagements_uuid UNIQUE (uuid),
    CONSTRAINT uq_engagements_org_id UNIQUE (organization_id, id),
    CONSTRAINT chk_engagements_status CHECK (status IN ('active', 'ended')),
    CONSTRAINT chk_engagements_ended CHECK ((status = 'ended') = (ended_at IS NOT NULL))
);

-- D1: bir mükellefin aynı anda (bütün ofisler genelinde) tek aktif engagement'ı olur.
CREATE UNIQUE INDEX uq_engagements_active_taxpayer
    ON engagements (taxpayer_id)
    WHERE ended_at IS NULL;

CREATE INDEX idx_engagements_organization_id ON engagements (organization_id, created_at DESC);

CREATE TRIGGER trg_engagements_set_updated_at
    BEFORE UPDATE ON engagements
    FOR EACH ROW
    EXECUTE FUNCTION set_updated_at();

-- engagement_assignments: ofis üyesi <-> engagement. Üye ofisten çıkınca
-- (organization_members satırı silinince) atamaları da silinir.
CREATE TABLE engagement_assignments (
    id              BIGSERIAL PRIMARY KEY,
    organization_id BIGINT      NOT NULL,
    engagement_id   BIGINT      NOT NULL,
    user_id         BIGINT      NOT NULL,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_engagement_assignments UNIQUE (engagement_id, user_id),
    CONSTRAINT fk_engagement_assignments_engagement
        FOREIGN KEY (organization_id, engagement_id)
        REFERENCES engagements (organization_id, id) ON DELETE CASCADE,
    CONSTRAINT fk_engagement_assignments_member
        FOREIGN KEY (organization_id, user_id)
        REFERENCES organization_members (organization_id, user_id) ON DELETE CASCADE
);

CREATE INDEX idx_engagement_assignments_org_user ON engagement_assignments (organization_id, user_id);

-- İzinler. read_all: atama olmadan ofisin bütün engagement'larına erişim.
INSERT INTO permissions (name, slug) VALUES
    ('Read tenant engagements', 'tenant.engagements.read'),
    ('Write tenant engagements', 'tenant.engagements.write'),
    ('Read all tenant engagements', 'tenant.engagements.read_all')
ON CONFLICT (slug) DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM roles r
JOIN permissions p ON p.slug IN ('tenant.engagements.read', 'tenant.engagements.write', 'tenant.engagements.read_all')
WHERE r.slug IN ('owner', 'admin') AND r.scope = 'tenant' AND r.organization_id IS NULL
ON CONFLICT DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM roles r
JOIN permissions p ON p.slug = 'tenant.engagements.read'
WHERE r.slug = 'member' AND r.scope = 'tenant' AND r.organization_id IS NULL
ON CONFLICT DO NOTHING;

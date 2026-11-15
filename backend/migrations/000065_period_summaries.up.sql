-- AI dönem özeti (issue #35, 06 T15, docs/00 K10): mükellefe sunulabilecek
-- finansal yorum taslağı. Rakamlar olgulardan (facts) gelir; AI yalnızca
-- {{F1}} gibi yer tutucularla metin yazar. Müşavir düzenler ve onaylar;
-- otomatik gönderim yok.
CREATE TABLE period_summaries (
    id              BIGSERIAL    PRIMARY KEY,
    uuid            UUID         NOT NULL DEFAULT gen_random_uuid(),
    organization_id BIGINT       NOT NULL,
    engagement_id   BIGINT       NOT NULL,
    taxpayer_id     BIGINT       NOT NULL REFERENCES taxpayers (id) ON DELETE RESTRICT,
    period          CHAR(7)      NOT NULL,
    status          TEXT         NOT NULL DEFAULT 'draft',
    -- Olgular: [{id, label, value, unit, rendered, source}]
    facts           JSONB        NOT NULL,
    -- AI'ın yer tutuculu metni ve olgularla doldurulmuş, düzenlenebilir metin.
    template        TEXT         NOT NULL,
    body            TEXT         NOT NULL,
    edited          BOOLEAN      NOT NULL DEFAULT false,
    model           TEXT         NULL,
    created_by      BIGINT       NULL REFERENCES users (id) ON DELETE SET NULL,
    approved_by     BIGINT       NULL REFERENCES users (id) ON DELETE SET NULL,
    approved_at     TIMESTAMPTZ  NULL,
    created_at      TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_period_summaries_uuid UNIQUE (uuid),
    CONSTRAINT fk_period_summaries_engagement
        FOREIGN KEY (organization_id, engagement_id) REFERENCES engagements (organization_id, id) ON DELETE CASCADE,
    CONSTRAINT chk_period_summaries_status CHECK (status IN ('draft', 'approved', 'superseded')),
    CONSTRAINT chk_period_summaries_period CHECK (period ~ '^[0-9]{4}-(0[1-9]|1[0-2])$'),
    CONSTRAINT chk_period_summaries_facts CHECK (jsonb_typeof(facts) = 'array')
);

CREATE UNIQUE INDEX uq_period_summaries_draft ON period_summaries (engagement_id, period) WHERE status = 'draft';
CREATE INDEX idx_period_summaries_list ON period_summaries (organization_id, engagement_id, period);

CREATE TRIGGER trg_period_summaries_set_updated_at
    BEFORE UPDATE ON period_summaries FOR EACH ROW EXECUTE FUNCTION set_updated_at();

SELECT rls_apply_engagement('period_summaries'::regclass);

INSERT INTO permissions (name, slug) VALUES
    ('Read tenant period summaries', 'tenant.summaries.read'),
    ('Write tenant period summaries', 'tenant.summaries.write'),
    ('Approve tenant period summaries', 'tenant.summaries.approve')
ON CONFLICT (slug) DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM roles r
JOIN permissions p ON p.slug IN ('tenant.summaries.read', 'tenant.summaries.write', 'tenant.summaries.approve')
WHERE r.slug IN ('owner', 'admin') AND r.scope = 'tenant' AND r.organization_id IS NULL
ON CONFLICT DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM roles r
JOIN permissions p ON p.slug IN ('tenant.summaries.read', 'tenant.summaries.write')
WHERE r.slug = 'member' AND r.scope = 'tenant' AND r.organization_id IS NULL
ON CONFLICT DO NOTHING;

-- Dilekçe üretimi (issue #56, T16). Şablonlar kodda; burada üretilen
-- dilekçelerin kaydı tutulur (ofis + engagement kapsamı: ofisin iş ürünü).
-- Kayıt, hangi şablon sürümüyle ve hangi alan değerleriyle üretildiğini ve
-- üretim anındaki metni saklar; PDF bu metinden yeniden basılır.
CREATE TABLE petitions (
    id               BIGSERIAL    PRIMARY KEY,
    uuid             UUID         NOT NULL DEFAULT gen_random_uuid(),
    organization_id  BIGINT       NOT NULL,
    engagement_id    BIGINT       NOT NULL,
    template_key     VARCHAR(64)  NOT NULL,
    template_version INTEGER      NOT NULL,
    fields           JSONB        NOT NULL DEFAULT '{}'::jsonb,
    rendered         JSONB        NOT NULL,
    draft            BOOLEAN      NOT NULL,
    created_by       BIGINT       NULL REFERENCES users (id) ON DELETE SET NULL,
    created_at       TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_petitions_uuid UNIQUE (uuid),
    CONSTRAINT fk_petitions_engagement
        FOREIGN KEY (organization_id, engagement_id)
        REFERENCES engagements (organization_id, id) ON DELETE CASCADE
);

CREATE INDEX idx_petitions_engagement ON petitions (organization_id, engagement_id, created_at DESC);

SELECT rls_apply_engagement('petitions');

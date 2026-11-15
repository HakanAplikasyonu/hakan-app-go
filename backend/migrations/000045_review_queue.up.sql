-- İnceleme kuyruğu ve köken kaydı (issue #14, docs/01 §3, G12).
--
-- suggestions: her öneri (hesap kodu, eşleşme, OCR alanı ...) güven skoru,
-- gerekçe ve kökenini (belge / kural / model / girdi) taşır. Eşik altı
-- öneriler müşavirin kuyruğuna düşer; eşik üstü olanlar otomatik uygulanır
-- ve geri alınabilir. Ofis iş ürünü: organization_id + engagement_id.
CREATE TABLE suggestions (
    id                 BIGSERIAL     PRIMARY KEY,
    uuid               UUID          NOT NULL DEFAULT gen_random_uuid(),
    organization_id    BIGINT        NOT NULL,
    engagement_id      BIGINT        NOT NULL,
    taxpayer_id        BIGINT        NOT NULL REFERENCES taxpayers (id) ON DELETE RESTRICT,
    kind               VARCHAR(64)   NOT NULL,
    target_type        VARCHAR(64)   NOT NULL,
    target_id          BIGINT        NULL,
    target_field       VARCHAR(64)   NOT NULL DEFAULT '',
    period             CHAR(7)       NULL,
    proposed_value     JSONB         NOT NULL,
    confidence         NUMERIC(5,4)  NOT NULL,
    rationale          TEXT          NOT NULL,
    -- Köken (G12): birincil kaynak ve ayrıntıları.
    origin             TEXT          NOT NULL,
    source_entity_type VARCHAR(64)   NULL,
    source_entity_id   BIGINT        NULL,
    rule_ref           VARCHAR(128)  NULL,
    model              VARCHAR(128)  NULL,
    ai_usage_event_id  BIGINT        NULL REFERENCES ai_usage_events (id) ON DELETE SET NULL,
    inputs             JSONB         NOT NULL DEFAULT '{}'::jsonb,
    status             TEXT          NOT NULL DEFAULT 'pending',
    applied_value      JSONB         NULL,
    previous_value     JSONB         NULL,
    -- Müşavir düzeltmesi: kural motoru bunu kural adayı olarak okur.
    rule_candidate     BOOLEAN       NOT NULL DEFAULT false,
    decided_by         BIGINT        NULL REFERENCES users (id) ON DELETE SET NULL,
    decided_at         TIMESTAMPTZ   NULL,
    created_at         TIMESTAMPTZ   NOT NULL DEFAULT NOW(),
    updated_at         TIMESTAMPTZ   NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_suggestions_uuid UNIQUE (uuid),
    CONSTRAINT uq_suggestions_org_id UNIQUE (organization_id, id),
    CONSTRAINT fk_suggestions_engagement
        FOREIGN KEY (organization_id, engagement_id) REFERENCES engagements (organization_id, id) ON DELETE CASCADE,
    CONSTRAINT chk_suggestions_kind CHECK (kind ~ '^[a-z][a-z0-9_]*$'),
    CONSTRAINT chk_suggestions_target_type CHECK (target_type ~ '^[a-z][a-z0-9_]*$'),
    CONSTRAINT chk_suggestions_confidence CHECK (confidence >= 0 AND confidence <= 1),
    CONSTRAINT chk_suggestions_rationale CHECK (btrim(rationale) <> ''),
    CONSTRAINT chk_suggestions_origin CHECK (origin IN ('document', 'rule', 'model', 'input')),
    CONSTRAINT chk_suggestions_origin_detail CHECK (
        (origin <> 'document' OR (source_entity_type IS NOT NULL AND source_entity_id IS NOT NULL))
        AND (origin <> 'rule' OR rule_ref IS NOT NULL)
        AND (origin <> 'model' OR model IS NOT NULL)
    ),
    CONSTRAINT chk_suggestions_inputs CHECK (jsonb_typeof(inputs) = 'object'),
    CONSTRAINT chk_suggestions_status CHECK (status IN (
        'pending', 'auto_accepted', 'accepted', 'corrected', 'rejected', 'reverted', 'superseded')),
    CONSTRAINT chk_suggestions_period CHECK (period IS NULL OR period ~ '^[0-9]{4}-(0[1-9]|1[0-2])$')
);

-- Aynı hedef alan için tek bekleyen öneri; yenisi eskisini superseded yapar.
CREATE UNIQUE INDEX uq_suggestions_pending
    ON suggestions (engagement_id, kind, target_type, COALESCE(target_id, 0), target_field)
    WHERE status = 'pending';
CREATE INDEX idx_suggestions_queue ON suggestions (organization_id, engagement_id, status, confidence);
CREATE INDEX idx_suggestions_rule_candidates ON suggestions (organization_id, kind) WHERE rule_candidate;

CREATE TRIGGER trg_suggestions_set_updated_at
    BEFORE UPDATE ON suggestions FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- suggestion_decisions: her kararın kim / ne zaman / neden kaydı. Yalnızca
-- eklenir; otomatik kabulde actor_user_id NULL, gerekçe eşik karşılaştırması.
CREATE TABLE suggestion_decisions (
    id              BIGSERIAL     PRIMARY KEY,
    organization_id BIGINT        NOT NULL,
    engagement_id   BIGINT        NOT NULL,
    suggestion_id   BIGINT        NOT NULL,
    action          TEXT          NOT NULL,
    actor_user_id   BIGINT        NULL REFERENCES users (id) ON DELETE SET NULL,
    reason          TEXT          NOT NULL DEFAULT '',
    value           JSONB         NULL,
    confidence      NUMERIC(5,4)  NOT NULL,
    threshold       NUMERIC(5,4)  NULL,
    created_at      TIMESTAMPTZ   NOT NULL DEFAULT NOW(),
    CONSTRAINT fk_suggestion_decisions_suggestion
        FOREIGN KEY (organization_id, suggestion_id) REFERENCES suggestions (organization_id, id) ON DELETE CASCADE,
    CONSTRAINT fk_suggestion_decisions_engagement
        FOREIGN KEY (organization_id, engagement_id) REFERENCES engagements (organization_id, id) ON DELETE CASCADE,
    CONSTRAINT chk_suggestion_decisions_action CHECK (action IN (
        'auto_accept', 'accept', 'reject', 'correct', 'revert', 'supersede'))
);

CREATE INDEX idx_suggestion_decisions_suggestion ON suggestion_decisions (suggestion_id, id);

-- review_settings: otomatik kabul eşiği, ofis (engagement_id NULL) ve
-- mükellef bazında; kind '*' bütün öneri türleri. NULL alan üst seviyeden.
CREATE TABLE review_settings (
    id              BIGSERIAL     PRIMARY KEY,
    organization_id BIGINT        NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    engagement_id   BIGINT        NULL,
    kind            VARCHAR(64)   NOT NULL DEFAULT '*',
    auto_accept     BOOLEAN       NULL,
    threshold       NUMERIC(5,4)  NULL,
    updated_by      BIGINT        NULL REFERENCES users (id) ON DELETE SET NULL,
    created_at      TIMESTAMPTZ   NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ   NOT NULL DEFAULT NOW(),
    CONSTRAINT fk_review_settings_engagement
        FOREIGN KEY (organization_id, engagement_id) REFERENCES engagements (organization_id, id) ON DELETE CASCADE,
    CONSTRAINT chk_review_settings_kind CHECK (kind = '*' OR kind ~ '^[a-z][a-z0-9_]*$'),
    CONSTRAINT chk_review_settings_threshold CHECK (threshold IS NULL OR (threshold >= 0 AND threshold <= 1))
);

CREATE UNIQUE INDEX uq_review_settings_office ON review_settings (organization_id, kind) WHERE engagement_id IS NULL;
CREATE UNIQUE INDEX uq_review_settings_engagement ON review_settings (organization_id, engagement_id, kind) WHERE engagement_id IS NOT NULL;

CREATE TRIGGER trg_review_settings_set_updated_at
    BEFORE UPDATE ON review_settings FOR EACH ROW EXECUTE FUNCTION set_updated_at();

INSERT INTO permissions (name, slug) VALUES
    ('Read tenant review queue', 'tenant.review.read'),
    ('Decide tenant suggestions', 'tenant.review.decide'),
    ('Write tenant review settings', 'tenant.review.settings.write')
ON CONFLICT (slug) DO NOTHING;

-- owner/admin: hepsi. member: kuyruğu görür ve karar verir, eşiği değiştiremez.
INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM roles r
JOIN permissions p ON p.slug IN ('tenant.review.read', 'tenant.review.decide', 'tenant.review.settings.write')
WHERE r.slug IN ('owner', 'admin') AND r.scope = 'tenant' AND r.organization_id IS NULL
ON CONFLICT DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM roles r
JOIN permissions p ON p.slug IN ('tenant.review.read', 'tenant.review.decide')
WHERE r.slug = 'member' AND r.scope = 'tenant' AND r.organization_id IS NULL
ON CONFLICT DO NOTHING;

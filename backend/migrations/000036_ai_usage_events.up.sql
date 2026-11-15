-- AI call cost ledger (docs/01 G11, issue #24). One row per successful
-- provider call, stamped with the office and, when known, the taxpayer it
-- was made for. cost_usd is an estimate from the model price table; NULL when
-- the model has no known price (self-hosted fallback).
CREATE TABLE ai_usage_events (
    id                 BIGSERIAL PRIMARY KEY,
    uuid               UUID          NOT NULL DEFAULT gen_random_uuid(),
    organization_id    BIGINT        NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    -- Taxpayer (mükellef) the call was made for. No FK yet: the taxpayer
    -- table lands with the shared data model (#9); the FK is added there.
    taxpayer_id        BIGINT        NULL,
    user_id            BIGINT        NULL REFERENCES users (id) ON DELETE SET NULL,
    purpose            VARCHAR(64)   NOT NULL,
    provider           VARCHAR(32)   NOT NULL,
    model              VARCHAR(128)  NOT NULL,
    is_fallback        BOOLEAN       NOT NULL DEFAULT false,
    input_tokens       BIGINT        NOT NULL DEFAULT 0,
    output_tokens      BIGINT        NOT NULL DEFAULT 0,
    cache_read_tokens  BIGINT        NOT NULL DEFAULT 0,
    cache_write_tokens BIGINT        NOT NULL DEFAULT 0,
    cost_usd           NUMERIC(14,6) NULL,
    masked_values      INTEGER       NOT NULL DEFAULT 0,
    created_at         TIMESTAMPTZ   NOT NULL DEFAULT now(),
    CONSTRAINT ai_usage_events_uuid_key UNIQUE (uuid),
    CONSTRAINT ai_usage_events_tokens_check CHECK (
        input_tokens >= 0 AND output_tokens >= 0
        AND cache_read_tokens >= 0 AND cache_write_tokens >= 0
    )
);

CREATE INDEX ai_usage_events_org_created_idx ON ai_usage_events (organization_id, created_at);
CREATE INDEX ai_usage_events_org_taxpayer_idx ON ai_usage_events (organization_id, taxpayer_id, created_at)
    WHERE taxpayer_id IS NOT NULL;

INSERT INTO permissions (name, slug) VALUES
    ('Read tenant AI usage', 'tenant.ai.usage.read')
ON CONFLICT (slug) DO NOTHING;

-- Owner and admin template roles see the office's AI cost report.
INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM roles r
JOIN permissions p ON p.slug = 'tenant.ai.usage.read'
WHERE r.slug IN ('owner', 'admin') AND r.scope = 'tenant' AND r.organization_id IS NULL
ON CONFLICT DO NOTHING;

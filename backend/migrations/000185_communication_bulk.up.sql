-- İletişim merkezi (#780 B bölümü): mükellef grupları ve elle toplu SMS /
-- e-posta gönderimi.
--
-- taxpayer_groups: ofisin ad verdiği sabit mükellef listeleri ("KDV'ye
-- hassas 10 mükellef"). Üyeler engagement (ofis <-> mükellef ilişkisi)
-- üzerinden tutulur; ilişki biterse üyelik de düşer.
CREATE TABLE taxpayer_groups (
    id               BIGSERIAL    PRIMARY KEY,
    uuid             UUID         NOT NULL DEFAULT gen_random_uuid(),
    organization_id  BIGINT       NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    name             TEXT         NOT NULL,
    description      TEXT         NOT NULL DEFAULT '',
    created_by       BIGINT       NULL REFERENCES users (id) ON DELETE SET NULL,
    created_at       TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at       TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_taxpayer_groups_uuid UNIQUE (uuid),
    CONSTRAINT uq_taxpayer_groups_org_id UNIQUE (organization_id, id),
    CONSTRAINT chk_taxpayer_groups_name CHECK (char_length(btrim(name)) BETWEEN 1 AND 80),
    CONSTRAINT chk_taxpayer_groups_description CHECK (char_length(description) <= 500)
);
-- Aynı ofiste aynı adlı iki grup olmaz (büyük/küçük harf fark etmez).
CREATE UNIQUE INDEX uq_taxpayer_groups_org_name ON taxpayer_groups (organization_id, lower(name));
CREATE INDEX idx_taxpayer_groups_created_by ON taxpayer_groups (created_by);
SELECT rls_apply_org('taxpayer_groups');

CREATE TABLE taxpayer_group_members (
    organization_id  BIGINT       NOT NULL,
    group_id         BIGINT       NOT NULL,
    engagement_id    BIGINT       NOT NULL,
    created_at       TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    CONSTRAINT pk_taxpayer_group_members PRIMARY KEY (group_id, engagement_id),
    CONSTRAINT fk_taxpayer_group_members_group FOREIGN KEY (organization_id, group_id)
        REFERENCES taxpayer_groups (organization_id, id) ON DELETE CASCADE,
    CONSTRAINT fk_taxpayer_group_members_engagement FOREIGN KEY (organization_id, engagement_id)
        REFERENCES engagements (organization_id, id) ON DELETE CASCADE
);
CREATE INDEX idx_taxpayer_group_members_group ON taxpayer_group_members (organization_id, group_id);
CREATE INDEX idx_taxpayer_group_members_engagement ON taxpayer_group_members (organization_id, engagement_id);
SELECT rls_apply_org('taxpayer_group_members');

-- bulk_sends: bir elle toplu gönderim. Mesajların kendisi sms_messages /
-- mail_messages içinde bulk_id = uuid ile durur (kanıt); burada ilerleme,
-- elenen tekrarlar ve iletişim bilgisi olmadığı için atlanan mükellefler
-- tutulur. engagement_ids hedefin çözülmüş hali: "Son gönderilenler"
-- kümesi bunu yeniden hedef olarak kullanır.
--
-- Durum: running (gönderiliyor), done (bitti), failed (yarıda kesildi).
CREATE TABLE bulk_sends (
    id               BIGSERIAL    PRIMARY KEY,
    uuid             UUID         NOT NULL DEFAULT gen_random_uuid(),
    organization_id  BIGINT       NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    channel          TEXT         NOT NULL,
    subject          TEXT         NOT NULL DEFAULT '',
    body             TEXT         NOT NULL,
    target_kind      TEXT         NOT NULL,
    target_label     TEXT         NOT NULL DEFAULT '',
    engagement_ids   BIGINT[]     NOT NULL DEFAULT '{}',
    status           TEXT         NOT NULL DEFAULT 'running',
    total            INTEGER      NOT NULL DEFAULT 0,
    processed        INTEGER      NOT NULL DEFAULT 0,
    sent             INTEGER      NOT NULL DEFAULT 0,
    failed           INTEGER      NOT NULL DEFAULT 0,
    duplicates       INTEGER      NOT NULL DEFAULT 0,
    -- [{"engagement_uuid","title","reason"}]: iletişim bilgisi yok / hatalı.
    skipped          JSONB        NOT NULL DEFAULT '[]',
    -- [{"title","recipient","status","error_message"}]: kayıt bile
    -- yazılamayan hatalar (ör. kampanya kelimesi). Kayıtlı hatalar
    -- sms_messages / mail_messages içinde durur.
    failures         JSONB        NOT NULL DEFAULT '[]',
    error_message    TEXT         NOT NULL DEFAULT '',
    created_by       BIGINT       NULL REFERENCES users (id) ON DELETE SET NULL,
    created_at       TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at       TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    finished_at      TIMESTAMPTZ  NULL,
    CONSTRAINT uq_bulk_sends_uuid UNIQUE (uuid),
    CONSTRAINT chk_bulk_sends_channel CHECK (channel IN ('sms', 'mail')),
    CONSTRAINT chk_bulk_sends_target CHECK (target_kind IN ('selection', 'group', 'filter', 'recent')),
    CONSTRAINT chk_bulk_sends_status CHECK (status IN ('running', 'done', 'failed')),
    CONSTRAINT chk_bulk_sends_total CHECK (total BETWEEN 0 AND 500),
    CONSTRAINT chk_bulk_sends_skipped CHECK (jsonb_typeof(skipped) = 'array'),
    CONSTRAINT chk_bulk_sends_failures CHECK (jsonb_typeof(failures) = 'array')
);
CREATE INDEX idx_bulk_sends_org_created ON bulk_sends (organization_id, created_at DESC);
-- Kullanıcı başına son gönderimler ("Son gönderilenler") ve users FK'si.
CREATE INDEX idx_bulk_sends_created_by ON bulk_sends (created_by, created_at DESC);
SELECT rls_apply_org('bulk_sends');

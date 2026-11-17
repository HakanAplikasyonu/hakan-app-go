-- WhatsApp Web uzantısı (issue #388): ofis kullanıcısı WhatsApp Web'de gelen
-- görsel / PDF'i tek tıkla gider, gelir ya da belge olarak sisteme alır.
--
-- whatsapp_chat_links: sohbet → mükellef (engagement) eşlemesi. Uzantı bir
-- sohbette ilk kez kayıt yapılırken mükellef aranıp seçilir; eşleme ofis
-- içinde hatırlanır, sonraki kayıtlarda seçim sorulmaz. chat_key sohbetin
-- WhatsApp kimliğidir (kişi: yalnızca rakamlardan telefon, grup: "g:" +
-- grup kimliği, numarası gizli kişi: "l:" + LID). Mesaj içeriği hiçbir
-- tabloda tutulmaz.
CREATE TABLE whatsapp_chat_links (
    id              BIGSERIAL    PRIMARY KEY,
    uuid            UUID         NOT NULL DEFAULT gen_random_uuid(),
    organization_id BIGINT       NOT NULL,
    engagement_id   BIGINT       NOT NULL,
    chat_key        TEXT         NOT NULL,
    label           TEXT         NOT NULL DEFAULT '',
    created_by      BIGINT       NULL REFERENCES users (id) ON DELETE SET NULL,
    updated_by      BIGINT       NULL REFERENCES users (id) ON DELETE SET NULL,
    created_at      TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_whatsapp_chat_links_uuid UNIQUE (uuid),
    CONSTRAINT uq_whatsapp_chat_links_chat UNIQUE (organization_id, chat_key),
    CONSTRAINT fk_whatsapp_chat_links_engagement
        FOREIGN KEY (organization_id, engagement_id) REFERENCES engagements (organization_id, id) ON DELETE CASCADE,
    CONSTRAINT chk_whatsapp_chat_links_key CHECK (chat_key ~ '^([0-9]{6,20}|[gl]:[0-9A-Za-z-]{6,64})$'),
    CONSTRAINT chk_whatsapp_chat_links_label CHECK (char_length(label) <= 120)
);

CREATE INDEX idx_whatsapp_chat_links_engagement ON whatsapp_chat_links (organization_id, engagement_id);

CREATE TRIGGER trg_whatsapp_chat_links_set_updated_at
    BEFORE UPDATE ON whatsapp_chat_links FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- whatsapp_captures: uzantıdan gelen her kaydın izi. Gider / gelir fiş
-- yakalamaya (receipt_captures, AI okuma → taslak) gider; belge, mükellefin
-- açık belge görevine (document_tasks) yüklenir ya da görevsiz ek olarak
-- (attachments) saklanır. Aynı dosya aynı hedefe ikinci kez gelirse yeni
-- satır açılmaz (uq_whatsapp_captures_file).
CREATE TABLE whatsapp_captures (
    id                 BIGSERIAL    PRIMARY KEY,
    uuid               UUID         NOT NULL DEFAULT gen_random_uuid(),
    organization_id    BIGINT       NOT NULL,
    engagement_id      BIGINT       NOT NULL,
    taxpayer_id        BIGINT       NOT NULL REFERENCES taxpayers (id) ON DELETE RESTRICT,
    target             TEXT         NOT NULL,
    chat_key           TEXT         NULL,
    file_name          TEXT         NOT NULL DEFAULT '',
    content_type       TEXT         NOT NULL,
    size_bytes         BIGINT       NOT NULL,
    sha256             CHAR(64)     NOT NULL,
    attachment_id      BIGINT       NULL,
    receipt_capture_id BIGINT       NULL REFERENCES receipt_captures (id) ON DELETE SET NULL,
    document_task_id   BIGINT       NULL REFERENCES document_tasks (id) ON DELETE SET NULL,
    created_by         BIGINT       NULL REFERENCES users (id) ON DELETE SET NULL,
    created_at         TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_whatsapp_captures_uuid UNIQUE (uuid),
    CONSTRAINT uq_whatsapp_captures_file UNIQUE (organization_id, engagement_id, target, sha256),
    CONSTRAINT fk_whatsapp_captures_engagement
        FOREIGN KEY (organization_id, engagement_id) REFERENCES engagements (organization_id, id) ON DELETE CASCADE,
    CONSTRAINT fk_whatsapp_captures_attachment FOREIGN KEY (taxpayer_id, attachment_id) REFERENCES attachments (taxpayer_id, id),
    CONSTRAINT chk_whatsapp_captures_target CHECK (target IN ('expense', 'income', 'document')),
    CONSTRAINT chk_whatsapp_captures_sha256 CHECK (sha256 ~ '^[0-9a-f]{64}$'),
    CONSTRAINT chk_whatsapp_captures_size CHECK (size_bytes > 0)
);

CREATE INDEX idx_whatsapp_captures_list ON whatsapp_captures (organization_id, created_at DESC);
CREATE INDEX idx_whatsapp_captures_engagement ON whatsapp_captures (organization_id, engagement_id, created_at DESC);

-- RLS (issue #6, #166): engagement kapsamlı ve baştan zorlanmış; bütün
-- erişim istek kapsamı ya da rls.System içinden.
SELECT rls_apply_engagement(t) FROM unnest(ARRAY['whatsapp_chat_links', 'whatsapp_captures']::regclass[]) AS t;
SELECT rls_enforce(t) FROM unnest(ARRAY['whatsapp_chat_links', 'whatsapp_captures']::regclass[]) AS t;

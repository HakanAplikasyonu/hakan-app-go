-- Eksik belge görevi (issue #36, docs/01 §3 Basis dersi, B2B2C).
--
-- Akış: kontrol (control:missing_document) bankada olup belgesi olmayan
-- hareket için BNK_UNDOCUMENTED_PAYMENT / BNK_UNINVOICED_COLLECTION uyarısı
-- üretir → ofis açık uyarılardan mükellefe görev açar → mükellef uygulamadan
-- fotoğraf / PDF yükler → belge hareketle eşlenir → kontrol yeniden çalışır,
-- uyarı auto_resolved olur. Açık görevler için hatırlatma bildirimi gider.
--
-- Banka tablolarına dokunulmaz (banking modülü #20'nin); eşleşme
-- bank_transaction_documents'ta tutulur.

-- ---------------------------------------------------------------------------
-- bank_transaction_documents: hareket ↔ belge (attachment) eşleşmesi.
-- Mükellefin kaynak verisidir (taxpayer_id NOT NULL, taxpayer_tables.txt):
-- mükellef ofis değiştirse de "bu ödemenin belgesi var" bilgisi kalır.
-- Hangi ofisin görevinden geldiği burada tutulmaz (D1b, D2).
-- ---------------------------------------------------------------------------
CREATE TABLE bank_transaction_documents (
    id                   BIGSERIAL    PRIMARY KEY,
    uuid                 UUID         NOT NULL DEFAULT gen_random_uuid(),
    taxpayer_id          BIGINT       NOT NULL REFERENCES taxpayers (id) ON DELETE RESTRICT,
    bank_transaction_id  BIGINT       NOT NULL,
    attachment_id        BIGINT       NOT NULL,
    -- taxpayer_upload: mükellef görev üzerinden yükledi.
    method               TEXT         NOT NULL DEFAULT 'taxpayer_upload',
    matched_by           BIGINT       NULL REFERENCES users (id) ON DELETE SET NULL,
    created_at           TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_bank_transaction_documents_uuid UNIQUE (uuid),
    CONSTRAINT uq_bank_transaction_documents_pair UNIQUE (taxpayer_id, bank_transaction_id, attachment_id),
    CONSTRAINT fk_bank_transaction_documents_transaction
        FOREIGN KEY (taxpayer_id, bank_transaction_id) REFERENCES bank_transactions (taxpayer_id, id) ON DELETE CASCADE,
    CONSTRAINT fk_bank_transaction_documents_attachment
        FOREIGN KEY (taxpayer_id, attachment_id) REFERENCES attachments (taxpayer_id, id) ON DELETE RESTRICT,
    CONSTRAINT chk_bank_transaction_documents_method CHECK (method IN ('taxpayer_upload', 'manual'))
);

CREATE INDEX idx_bank_transaction_documents_transaction
    ON bank_transaction_documents (taxpayer_id, bank_transaction_id);

-- ---------------------------------------------------------------------------
-- document_tasks: ofisin mükelleften belge isteği. Ofis iş ürünüdür
-- (organization_id + engagement_id, tenant_tables.txt "engagement").
-- Bir hareket için aynı engagement'ta en fazla bir açık görev olur.
--
-- status: open (belge bekleniyor) → matched (belge yüklendi ve eşlendi);
-- cancelled (ofis iptal etti ya da koşul başka yolla kalktı).
-- submitted: belge yüklendi ama otomatik eşlenemedi (ör. hareket iptal);
-- ofisin bakması gerekir.
-- ---------------------------------------------------------------------------
CREATE TABLE document_tasks (
    id                   BIGSERIAL    PRIMARY KEY,
    uuid                 UUID         NOT NULL DEFAULT gen_random_uuid(),
    organization_id      BIGINT       NOT NULL,
    engagement_id        BIGINT       NOT NULL,
    taxpayer_id          BIGINT       NOT NULL REFERENCES taxpayers (id) ON DELETE RESTRICT,
    bank_transaction_id  BIGINT       NOT NULL,
    -- Görevi doğuran uyarı (data_warnings.id). Uyarı yeniden açılırsa yeni
    -- satır olur; görev ilkine bağlı kalır.
    warning_id           BIGINT       NULL REFERENCES data_warnings (id) ON DELETE SET NULL,
    code                 TEXT         NOT NULL,
    kind                 TEXT         NOT NULL,
    period               CHAR(7)      NOT NULL,
    status               TEXT         NOT NULL DEFAULT 'open',
    -- Mükellefe gösterilen not (ofis yazar).
    note                 TEXT         NOT NULL DEFAULT '',
    due_at               TIMESTAMPTZ  NOT NULL,
    reminder_count       INTEGER      NOT NULL DEFAULT 0,
    last_reminded_at     TIMESTAMPTZ  NULL,
    -- NULL: hatırlatma bitti (üst sınır) ya da görev kapalı.
    next_reminder_at     TIMESTAMPTZ  NULL,
    created_by           BIGINT       NULL REFERENCES users (id) ON DELETE SET NULL,
    submitted_at         TIMESTAMPTZ  NULL,
    matched_at           TIMESTAMPTZ  NULL,
    cancelled_at         TIMESTAMPTZ  NULL,
    cancelled_by         BIGINT       NULL REFERENCES users (id) ON DELETE SET NULL,
    cancel_reason        TEXT         NULL,
    created_at           TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at           TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_document_tasks_uuid UNIQUE (uuid),
    CONSTRAINT uq_document_tasks_org_id UNIQUE (organization_id, id),
    CONSTRAINT fk_document_tasks_engagement
        FOREIGN KEY (organization_id, engagement_id) REFERENCES engagements (organization_id, id) ON DELETE CASCADE,
    CONSTRAINT fk_document_tasks_transaction
        FOREIGN KEY (taxpayer_id, bank_transaction_id) REFERENCES bank_transactions (taxpayer_id, id) ON DELETE CASCADE,
    CONSTRAINT chk_document_tasks_code CHECK (code IN ('BNK_UNDOCUMENTED_PAYMENT', 'BNK_UNINVOICED_COLLECTION')),
    CONSTRAINT chk_document_tasks_kind CHECK (kind IN ('undocumented_payment', 'uninvoiced_collection')),
    CONSTRAINT chk_document_tasks_status CHECK (status IN ('open', 'submitted', 'matched', 'cancelled')),
    CONSTRAINT chk_document_tasks_period CHECK (period ~ '^[0-9]{4}-(0[1-9]|1[0-2])$'),
    CONSTRAINT chk_document_tasks_reminders CHECK (reminder_count >= 0),
    CONSTRAINT chk_document_tasks_note CHECK (char_length(note) <= 1000)
);

CREATE UNIQUE INDEX uq_document_tasks_active
    ON document_tasks (engagement_id, bank_transaction_id)
    WHERE status IN ('open', 'submitted');
CREATE INDEX idx_document_tasks_list ON document_tasks (organization_id, engagement_id, status, period);
CREATE INDEX idx_document_tasks_taxpayer ON document_tasks (taxpayer_id, status);
CREATE INDEX idx_document_tasks_reminder ON document_tasks (next_reminder_at) WHERE status = 'open' AND next_reminder_at IS NOT NULL;

CREATE TRIGGER trg_document_tasks_set_updated_at
    BEFORE UPDATE ON document_tasks FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- ---------------------------------------------------------------------------
-- document_task_uploads: göreve yüklenen dosyalar (attachment bağlantısı).
-- Görevle aynı kapsam (organization_id + engagement_id).
-- ---------------------------------------------------------------------------
CREATE TABLE document_task_uploads (
    id               BIGSERIAL    PRIMARY KEY,
    uuid             UUID         NOT NULL DEFAULT gen_random_uuid(),
    organization_id  BIGINT       NOT NULL,
    engagement_id    BIGINT       NOT NULL,
    taxpayer_id      BIGINT       NOT NULL REFERENCES taxpayers (id) ON DELETE RESTRICT,
    task_id          BIGINT       NOT NULL,
    attachment_id    BIGINT       NOT NULL,
    uploaded_by      BIGINT       NULL REFERENCES users (id) ON DELETE SET NULL,
    created_at       TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_document_task_uploads_uuid UNIQUE (uuid),
    CONSTRAINT uq_document_task_uploads_pair UNIQUE (task_id, attachment_id),
    CONSTRAINT fk_document_task_uploads_task
        FOREIGN KEY (organization_id, task_id) REFERENCES document_tasks (organization_id, id) ON DELETE CASCADE,
    CONSTRAINT fk_document_task_uploads_engagement
        FOREIGN KEY (organization_id, engagement_id) REFERENCES engagements (organization_id, id) ON DELETE CASCADE,
    CONSTRAINT fk_document_task_uploads_attachment
        FOREIGN KEY (taxpayer_id, attachment_id) REFERENCES attachments (taxpayer_id, id) ON DELETE RESTRICT
);

CREATE INDEX idx_document_task_uploads_task ON document_task_uploads (organization_id, engagement_id, task_id);

-- ---------------------------------------------------------------------------
-- İzinler. Sahip yeni tenant.* iznini kendiliğinden alır (000044).
-- ---------------------------------------------------------------------------
-- RLS (issue #6).
SELECT rls_apply_taxpayer('bank_transaction_documents'::regclass);
SELECT rls_apply_engagement(t) FROM unnest(ARRAY['document_tasks', 'document_task_uploads']::regclass[]) AS t;

INSERT INTO permissions (name, slug) VALUES
    ('Read document tasks', 'tenant.doc_tasks.read'),
    ('Write document tasks', 'tenant.doc_tasks.write')
ON CONFLICT (slug) DO NOTHING;

SELECT grant_tenant_role_permission('admin', 'tenant.doc_tasks.read');
SELECT grant_tenant_role_permission('admin', 'tenant.doc_tasks.write');
SELECT grant_tenant_role_permission('accountant', 'tenant.doc_tasks.read');
SELECT grant_tenant_role_permission('accountant', 'tenant.doc_tasks.write');
SELECT grant_tenant_role_permission('member', 'tenant.doc_tasks.read');
SELECT grant_tenant_role_permission('member', 'tenant.doc_tasks.write');
SELECT grant_tenant_role_permission('intern', 'tenant.doc_tasks.read');

-- ---------------------------------------------------------------------------
-- Bildirim şablonları. Değişkenler: org_name, taxpayer_name, count, link.
-- inapp satırı mobil push'a da yansır.
-- ---------------------------------------------------------------------------
INSERT INTO notification_templates (code, channel, language, subject, body, active) VALUES
    ('doc_tasks.created', 'inapp', 'tr', 'Yeni belge talebi',
     '{{org_name}}, {{taxpayer_name}} için {{count}} banka hareketinin belgesini istiyor.', TRUE),
    ('doc_tasks.created', 'inapp', 'en', 'New document request',
     '{{org_name}} requests documents for {{count}} bank transactions of {{taxpayer_name}}.', TRUE),
    ('doc_tasks.created', 'email', 'tr', '{{org_name}} belge bekliyor',
     'Merhaba, {{org_name}} {{taxpayer_name}} için {{count}} banka hareketinin belgesini (fatura, fiş, dekont) istiyor. Uygulamadan fotoğrafını ya da PDF''ini yükleyebilirsiniz: {{link}}', TRUE),
    ('doc_tasks.created', 'email', 'en', '{{org_name}} is waiting for documents',
     'Hello, {{org_name}} requests documents (invoice, receipt) for {{count}} bank transactions of {{taxpayer_name}}. Upload a photo or PDF in the app: {{link}}', TRUE),
    ('doc_tasks.reminder', 'inapp', 'tr', 'Bekleyen belge talebi',
     '{{taxpayer_name}} için {{count}} belge hâlâ bekleniyor.', TRUE),
    ('doc_tasks.reminder', 'inapp', 'en', 'Pending document request',
     '{{count}} documents are still pending for {{taxpayer_name}}.', TRUE),
    ('doc_tasks.reminder', 'email', 'tr', 'Hatırlatma: {{count}} belge bekleniyor',
     'Merhaba, {{org_name}} {{taxpayer_name}} için {{count}} banka hareketinin belgesini hâlâ bekliyor. Yüklemek için: {{link}}', TRUE),
    ('doc_tasks.reminder', 'email', 'en', 'Reminder: {{count}} documents pending',
     'Hello, {{org_name}} is still waiting for documents for {{count}} bank transactions of {{taxpayer_name}}. Upload them here: {{link}}', TRUE)
ON CONFLICT (code, channel, language) DO NOTHING;

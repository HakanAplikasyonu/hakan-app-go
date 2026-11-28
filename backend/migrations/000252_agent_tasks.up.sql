-- Masaüstü yardımcısı görevleri (docs/tasarim/masaustu-luca-sgk.md §2.1).
-- Mali müşavirin bilgisayarındaki HAKAN masaüstü uygulaması, kullanıcının
-- kendi açtığı Luca oturumunda dosyaları Luca'nın içe aktarma ekranına
-- yükler. Luca şifresi, CAPTCHA ve doğrulama kodu sunucuya hiç gelmez.
--
-- Ofis ayarı: yardımcı varsayılan KAPALI (pilot ofiste açılır).
CREATE TABLE desktop_helper_settings (
    organization_id BIGINT       PRIMARY KEY REFERENCES organizations (id) ON DELETE CASCADE,
    luca_enabled    BOOLEAN      NOT NULL DEFAULT FALSE,
    sgk_enabled     BOOLEAN      NOT NULL DEFAULT FALSE,
    updated_by      BIGINT       NULL REFERENCES users (id) ON DELETE SET NULL,
    created_at      TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);
CREATE TRIGGER trg_desktop_helper_settings_set_updated_at
    BEFORE UPDATE ON desktop_helper_settings FOR EACH ROW EXECUTE FUNCTION set_updated_at();
SELECT rls_apply_org('desktop_helper_settings');

-- Bir satır = yardımcının bir mükellef için yaptığı bir iş.
--   luca_cari_sync:   cari sorgula, yoksa aç, kodu cari hafızasına yaz.
--   luca_bulk_export: "Luca'ya aktar" (dönemin banka, Z, fatura, masraf,
--                     yevmiye dosyaları). Dosyalar ajan görevi üstlenince
--                     üretilir (önce cari görevi biter, kodlar yazılır).
-- Aynı tıklamanın görevleri run_uuid ile bağlıdır ve sequence sırasıyla
-- yürür. approved_at boş görevi ajan alamaz (yeni cari tetiğiyle açılan
-- görev kullanıcı onaylayınca ya da bir aktarım sırasına girince çalışır).
--
-- Durumlar: queued → claimed → waiting_user (giriş / kod bekleniyor) ↔
-- running → done | partial | failed | needs_check | cancelled.
-- needs_check: "Yükle"den sonra sonuç okunamadı; asla otomatik yeniden
-- denenmez, kullanıcı Luca'ya bakıp kapatır (resolution).
CREATE TABLE agent_tasks (
    id               BIGSERIAL    PRIMARY KEY,
    uuid             UUID         NOT NULL DEFAULT gen_random_uuid(),
    organization_id  BIGINT       NOT NULL,
    engagement_id    BIGINT       NOT NULL,
    kind             TEXT         NOT NULL,
    run_uuid         UUID         NULL,
    sequence         INTEGER      NOT NULL DEFAULT 0,
    period           TEXT         NULL,
    -- Bulk görevde seçili türler (bank, z, incoming_invoices, …). Sır yok.
    kinds            TEXT[]       NOT NULL DEFAULT '{}',
    status           TEXT         NOT NULL DEFAULT 'queued',
    -- {step, steps, message}: "Dosya 2/4".
    progress         JSONB        NULL,
    -- Dosya üretiminde ayıklanan belgeler: {check, skipped, already_exported}.
    server_report    JSONB        NULL,
    error_code       TEXT         NULL,
    error_message    TEXT         NULL,
    resolution       TEXT         NULL,
    requested_by     BIGINT       NULL REFERENCES users (id) ON DELETE SET NULL,
    approved_by      BIGINT       NULL REFERENCES users (id) ON DELETE SET NULL,
    approved_at      TIMESTAMPTZ  NULL,
    resolved_by      BIGINT       NULL REFERENCES users (id) ON DELETE SET NULL,
    agent_id         BIGINT       NULL REFERENCES desktop_agents (id) ON DELETE SET NULL,
    claimed_at       TIMESTAMPTZ  NULL,
    finished_at      TIMESTAMPTZ  NULL,
    created_at       TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at       TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_agent_tasks_uuid UNIQUE (uuid),
    CONSTRAINT uq_agent_tasks_engagement_id UNIQUE (engagement_id, id),
    CONSTRAINT fk_agent_tasks_engagement
        FOREIGN KEY (organization_id, engagement_id) REFERENCES engagements (organization_id, id) ON DELETE CASCADE,
    CONSTRAINT chk_agent_tasks_kind CHECK (kind IN ('luca_cari_sync', 'luca_bulk_export')),
    CONSTRAINT chk_agent_tasks_status CHECK (status IN
        ('queued', 'claimed', 'waiting_user', 'running', 'done', 'partial', 'failed', 'needs_check', 'cancelled')),
    CONSTRAINT chk_agent_tasks_period CHECK (period IS NULL OR period ~ '^[0-9]{4}-(0[1-9]|1[0-2])$'),
    CONSTRAINT chk_agent_tasks_bulk_period CHECK (kind <> 'luca_bulk_export' OR period IS NOT NULL),
    CONSTRAINT chk_agent_tasks_resolution CHECK (resolution IS NULL OR resolution IN ('imported', 'not_imported')),
    CONSTRAINT chk_agent_tasks_error_code CHECK (error_code IS NULL OR error_code ~ '^[A-Z][A-Z0-9_]{1,63}$'),
    CONSTRAINT chk_agent_tasks_error CHECK (error_message IS NULL OR length(error_message) <= 2000)
);

-- Mükellefin yeni carileri onaysız tek görevde birikir.
CREATE UNIQUE INDEX uq_agent_tasks_cari_pending ON agent_tasks (engagement_id)
    WHERE kind = 'luca_cari_sync' AND status = 'queued' AND approved_at IS NULL;
-- Bir mükellef + dönemin tek açık aktarımı.
CREATE UNIQUE INDEX uq_agent_tasks_bulk_open ON agent_tasks (engagement_id, period)
    WHERE kind = 'luca_bulk_export' AND status IN ('queued', 'claimed', 'waiting_user', 'running');
CREATE INDEX idx_agent_tasks_list ON agent_tasks (organization_id, created_at DESC);
CREATE INDEX idx_agent_tasks_open ON agent_tasks (organization_id, status)
    WHERE status IN ('queued', 'claimed', 'waiting_user', 'running');
CREATE INDEX idx_agent_tasks_run ON agent_tasks (run_uuid, sequence) WHERE run_uuid IS NOT NULL;
CREATE INDEX idx_agent_tasks_agent ON agent_tasks (agent_id) WHERE agent_id IS NOT NULL;

CREATE TRIGGER trg_agent_tasks_set_updated_at
    BEFORE UPDATE ON agent_tasks FOR EACH ROW EXECUTE FUNCTION set_updated_at();
SELECT rls_apply_engagement('agent_tasks');

-- Cari görevinin satırları: VKN + ana hesap başına tek satır.
CREATE TABLE agent_task_counterparties (
    id               BIGSERIAL    PRIMARY KEY,
    organization_id  BIGINT       NOT NULL,
    engagement_id    BIGINT       NOT NULL,
    task_id          BIGINT       NOT NULL,
    tax_id           VARCHAR(11)  NOT NULL,
    main_code        CHAR(3)      NOT NULL,
    title            TEXT         NOT NULL DEFAULT '',
    status           TEXT         NOT NULL DEFAULT 'pending',
    account_code     VARCHAR(64)  NULL,
    -- conflict: HAKAN cari hafızasındaki (ezilmeyen) kod.
    hakan_account_code VARCHAR(64) NULL,
    message          TEXT         NULL,
    created_at       TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at       TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_agent_task_counterparties_key UNIQUE (task_id, tax_id, main_code),
    CONSTRAINT fk_agent_task_counterparties_engagement
        FOREIGN KEY (organization_id, engagement_id) REFERENCES engagements (organization_id, id) ON DELETE CASCADE,
    CONSTRAINT fk_agent_task_counterparties_task
        FOREIGN KEY (engagement_id, task_id) REFERENCES agent_tasks (engagement_id, id) ON DELETE CASCADE,
    CONSTRAINT chk_agent_task_counterparties_tax_id CHECK (tax_id ~ '^[0-9]{10,11}$'),
    CONSTRAINT chk_agent_task_counterparties_main CHECK (main_code ~ '^[0-9]{3}$'),
    CONSTRAINT chk_agent_task_counterparties_status CHECK (status IN ('pending', 'found', 'created', 'failed', 'conflict')),
    CONSTRAINT chk_agent_task_counterparties_code CHECK (account_code IS NULL OR account_code ~ '^[0-9]{3}(\.[0-9A-Za-z]+)*$'),
    CONSTRAINT chk_agent_task_counterparties_message CHECK (message IS NULL OR length(message) <= 500)
);
CREATE TRIGGER trg_agent_task_counterparties_set_updated_at
    BEFORE UPDATE ON agent_task_counterparties FOR EACH ROW EXECUTE FUNCTION set_updated_at();
CREATE INDEX idx_agent_task_counterparties_engagement ON agent_task_counterparties (organization_id, engagement_id);
SELECT rls_apply_engagement('agent_task_counterparties');

-- Aktarım görevinin dosyaları (Luca sınırına göre bölünmüş, ≤50 fiş).
CREATE TABLE agent_task_files (
    id               BIGSERIAL    PRIMARY KEY,
    uuid             UUID         NOT NULL DEFAULT gen_random_uuid(),
    organization_id  BIGINT       NOT NULL,
    engagement_id    BIGINT       NOT NULL,
    task_id          BIGINT       NOT NULL,
    sequence         INTEGER      NOT NULL,
    kind             TEXT         NOT NULL,
    name             TEXT         NOT NULL,
    content_type     TEXT         NOT NULL,
    size             BIGINT       NOT NULL,
    sha256           CHAR(64)     NOT NULL,
    storage_key      TEXT         NOT NULL,
    entries          INTEGER      NOT NULL DEFAULT 0,
    -- Liste / masraf dosyasında expense_export_documents.export_uuid.
    export_uuid      UUID         NULL,
    -- Yevmiye çıktısında method = 'luca_web' aktarım satırı.
    transfer_id      BIGINT       NULL REFERENCES accounting_transfers (id) ON DELETE SET NULL,
    -- [{document_key, document_no, document_date}]
    documents        JSONB        NOT NULL DEFAULT '[]'::jsonb,
    status           TEXT         NOT NULL DEFAULT 'pending',
    luca_message     TEXT         NULL,
    -- Hatalı satır raporu: [{entry_no, document_no, document_key, row, status, luca_entry_no, reason}]
    entry_results    JSONB        NOT NULL DEFAULT '[]'::jsonb,
    reported_at      TIMESTAMPTZ  NULL,
    created_at       TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at       TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_agent_task_files_uuid UNIQUE (uuid),
    CONSTRAINT uq_agent_task_files_seq UNIQUE (task_id, sequence),
    CONSTRAINT fk_agent_task_files_engagement
        FOREIGN KEY (organization_id, engagement_id) REFERENCES engagements (organization_id, id) ON DELETE CASCADE,
    CONSTRAINT fk_agent_task_files_task
        FOREIGN KEY (engagement_id, task_id) REFERENCES agent_tasks (engagement_id, id) ON DELETE CASCADE,
    CONSTRAINT chk_agent_task_files_kind CHECK (kind IN
        ('bank', 'z', 'incoming_invoices', 'outgoing_invoices', 'expenses', 'journals')),
    CONSTRAINT chk_agent_task_files_status CHECK (status IN ('pending', 'ok', 'failed', 'unknown')),
    CONSTRAINT chk_agent_task_files_sha CHECK (sha256 ~ '^[0-9a-f]{64}$'),
    CONSTRAINT chk_agent_task_files_message CHECK (luca_message IS NULL OR length(luca_message) <= 500)
);
CREATE TRIGGER trg_agent_task_files_set_updated_at
    BEFORE UPDATE ON agent_task_files FOR EACH ROW EXECUTE FUNCTION set_updated_at();
CREATE INDEX idx_agent_task_files_engagement ON agent_task_files (organization_id, engagement_id);
CREATE INDEX idx_agent_task_files_transfer ON agent_task_files (transfer_id) WHERE transfer_id IS NOT NULL;
SELECT rls_apply_engagement('agent_task_files');

-- Yevmiye çıktısının yardımcıyla Luca web ekranına yüklenmesi.
ALTER TABLE accounting_transfers DROP CONSTRAINT chk_accounting_transfers_method;
ALTER TABLE accounting_transfers ADD CONSTRAINT chk_accounting_transfers_method
    CHECK (method IN ('agent_folder', 'manual', 'luca_web'));

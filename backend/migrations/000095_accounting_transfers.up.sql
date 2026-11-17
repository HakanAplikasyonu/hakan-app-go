-- Muhasebe programına aktarım görevi (issue #391,
-- docs/arastirma/luca-zirve-aktarim.md §4). Bir satır = bir çıktının
-- (export_batches) bir programa bir kez gönderilmesi.
--
-- agent_folder: ofisin masaüstü ajanı çıktı dosyalarını mükellefin aktarım
-- klasörüne bırakır ve sonucu bildirir; kullanıcı programda içe aktarıp
-- görevi "aktarıldı" ya da "hata verdi" olarak kapatır. Luca / Zirve kimlik
-- bilgisi gerekmez ve saklanmaz.
--
-- Numara 000095: 000094 aynı anda başka bir şeritte kullanıldı. Bu dosya
-- 000094 olarak uygulanmış bir veritabanında da hatasız çalışır (IF NOT EXISTS).
--
-- Durumlar: queued → claimed (ajan üstlendi) → delivered (dosyalar
-- klasörde) → imported | failed. cancelled: kullanıcı vazgeçti. Yeniden
-- deneme yeni satır açar; eski hata geçmişte kalır.
CREATE TABLE IF NOT EXISTS accounting_transfers (
    id                BIGSERIAL    PRIMARY KEY,
    uuid              UUID         NOT NULL DEFAULT gen_random_uuid(),
    organization_id   BIGINT       NOT NULL,
    engagement_id     BIGINT       NOT NULL,
    export_batch_id   BIGINT       NOT NULL,
    driver            TEXT         NOT NULL,
    method            TEXT         NOT NULL DEFAULT 'agent_folder',
    status            TEXT         NOT NULL DEFAULT 'queued',
    agent_id          BIGINT       NULL REFERENCES desktop_agents (id) ON DELETE SET NULL,
    -- Ajanın klasöre bıraktığı dosyalar: [{name, sha256, size}]
    delivered_files   JSONB        NOT NULL DEFAULT '[]'::jsonb,
    -- Klasörün kullanıcıya gösterilen adı (tam yol değil).
    folder_label      TEXT         NULL,
    error_code        TEXT         NULL,
    error_message     TEXT         NULL,
    requested_by      BIGINT       NOT NULL REFERENCES users (id) ON DELETE RESTRICT,
    completed_by      BIGINT       NULL REFERENCES users (id) ON DELETE SET NULL,
    claimed_at        TIMESTAMPTZ  NULL,
    delivered_at      TIMESTAMPTZ  NULL,
    completed_at      TIMESTAMPTZ  NULL,
    created_at        TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at        TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_accounting_transfers_uuid UNIQUE (uuid),
    CONSTRAINT fk_accounting_transfers_engagement
        FOREIGN KEY (organization_id, engagement_id) REFERENCES engagements (organization_id, id) ON DELETE CASCADE,
    CONSTRAINT fk_accounting_transfers_batch
        FOREIGN KEY (engagement_id, export_batch_id) REFERENCES export_batches (engagement_id, id) ON DELETE CASCADE,
    CONSTRAINT chk_accounting_transfers_driver CHECK (driver ~ '^[a-z][a-z0-9_]*$'),
    CONSTRAINT chk_accounting_transfers_method CHECK (method IN ('agent_folder')),
    CONSTRAINT chk_accounting_transfers_status CHECK (status IN ('queued', 'claimed', 'delivered', 'imported', 'failed', 'cancelled')),
    CONSTRAINT chk_accounting_transfers_error CHECK (error_message IS NULL OR length(error_message) <= 2000),
    CONSTRAINT chk_accounting_transfers_folder CHECK (folder_label IS NULL OR length(folder_label) <= 200)
);

-- Bir çıktı için tek açık ya da aktarılmış görev: aynı fişler programa iki
-- kez gitmez (Luca aynı dosyayı iki kez yükleyince iki fiş keser).
CREATE UNIQUE INDEX IF NOT EXISTS uq_accounting_transfers_live ON accounting_transfers (export_batch_id)
    WHERE status IN ('queued', 'claimed', 'delivered', 'imported');
CREATE INDEX IF NOT EXISTS idx_accounting_transfers_list ON accounting_transfers (organization_id, engagement_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_accounting_transfers_pending ON accounting_transfers (engagement_id, status)
    WHERE status IN ('queued', 'claimed');

DROP TRIGGER IF EXISTS trg_accounting_transfers_set_updated_at ON accounting_transfers;
CREATE TRIGGER trg_accounting_transfers_set_updated_at
    BEFORE UPDATE ON accounting_transfers FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- RLS: engagement kapsamlı (ENABLE + FORCE + rls_engagement politikası).
SELECT rls_apply_engagement('accounting_transfers');

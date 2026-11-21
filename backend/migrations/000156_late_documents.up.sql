-- Geç gelen belge ve geçmiş dönem takibi (#780 C).
--
-- Belgenin iki ayrı tarihi vardır:
--   geliş tarihi     : belgenin sisteme girdiği an (kaynak senkronu, yükleme,
--                      e-posta). Belge tablolarının created_at sütunu.
--   ait olduğu dönem : belge tarihinden gelen ay (invoices.period,
--                      expenses.period, retail_sales.period).
--
-- Dönem kapanışı: dönemin kilitlendiği an (period_locks.locked_at) ya da o
-- dönemin bir beyannamesinin verildi olarak işaretlendiği an
-- (tax_returns.filed_at); hangisi önceyse. Kapanıştan sonra gelen belge
-- burada bir satır olur ve ofisin kararını bekler:
--   pending        bekliyor
--   correction     düzeltme beyannamesine alındı
--   current_period cari döneme alındı (target_period)
--   ignored        yok sayıldı
-- Satırları tarama yazar (aynı belge bir kez); karar satırda kalır.
CREATE TABLE late_documents (
    id              BIGSERIAL    PRIMARY KEY,
    uuid            UUID         NOT NULL DEFAULT gen_random_uuid(),
    organization_id BIGINT       NOT NULL,
    engagement_id   BIGINT       NOT NULL,
    document_type   TEXT         NOT NULL,
    -- Belge tablosundaki id ve uuid (mükellef kapsamlı tablolar).
    document_id     BIGINT       NOT NULL,
    document_uuid   UUID         NOT NULL,
    period          CHAR(7)      NOT NULL,
    arrived_at      TIMESTAMPTZ  NOT NULL,
    closed_at       TIMESTAMPTZ  NOT NULL,
    close_reason    TEXT         NOT NULL,
    status          TEXT         NOT NULL DEFAULT 'pending',
    target_period   CHAR(7)      NULL,
    note            TEXT         NULL,
    decided_by      BIGINT       NULL REFERENCES users (id) ON DELETE SET NULL,
    decided_at      TIMESTAMPTZ  NULL,
    created_at      TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_late_documents_uuid UNIQUE (uuid),
    CONSTRAINT uq_late_documents_document UNIQUE (engagement_id, document_type, document_id),
    CONSTRAINT fk_late_documents_engagement
        FOREIGN KEY (organization_id, engagement_id) REFERENCES engagements (organization_id, id) ON DELETE CASCADE,
    CONSTRAINT chk_late_documents_type CHECK (document_type IN ('invoice', 'expense', 'retail_sale')),
    CONSTRAINT chk_late_documents_period CHECK (period ~ '^[0-9]{4}-(0[1-9]|1[0-2])$'),
    CONSTRAINT chk_late_documents_close_reason CHECK (close_reason IN ('lock', 'filed')),
    CONSTRAINT chk_late_documents_status CHECK (status IN ('pending', 'correction', 'current_period', 'ignored')),
    CONSTRAINT chk_late_documents_target CHECK (
        (status = 'current_period') = (target_period IS NOT NULL)
        AND (target_period IS NULL OR (target_period ~ '^[0-9]{4}-(0[1-9]|1[0-2])$' AND target_period > period))
    ),
    CONSTRAINT chk_late_documents_decided CHECK ((status = 'pending') = (decided_at IS NULL))
);

CREATE INDEX idx_late_documents_list ON late_documents (organization_id, engagement_id, status, period);
CREATE INDEX idx_late_documents_document ON late_documents (organization_id, engagement_id, document_uuid);
CREATE INDEX idx_late_documents_decided_by ON late_documents (decided_by);

-- RLS (issue #6): engagement kapsamlı.
SELECT rls_apply_engagement('late_documents'::regclass);

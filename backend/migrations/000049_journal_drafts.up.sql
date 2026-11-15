-- Yevmiye taslağı ve muhasebe çıktısı (issue #26, docs/02 §2, §4).
-- Ofisin iş ürünüdür: organization_id + engagement_id; mükellef taxpayer_id'de
-- tekrar tutulur. Tutarlar NUMERIC(18,4).

-- Mükellef (engagement) bazında çıktı ayarları.
CREATE TABLE export_settings (
    id                            BIGSERIAL    PRIMARY KEY,
    organization_id               BIGINT       NOT NULL,
    engagement_id                 BIGINT       NOT NULL,
    -- TÜRMOB tercihli entegratör: faturalar Luca'ya zaten gidiyor; fatura
    -- kaynaklı fişler çıktıya varsayılan olarak girmez (docs/02 §4).
    invoices_reach_luca_directly  BOOLEAN      NOT NULL DEFAULT FALSE,
    -- Hedef hesap planında olmayan kod çıktıyı engeller mi (docs/02 §5 test 4).
    block_on_unknown_account      BOOLEAN      NOT NULL DEFAULT TRUE,
    updated_by                    BIGINT       NULL REFERENCES users (id) ON DELETE SET NULL,
    created_at                    TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at                    TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_export_settings_engagement UNIQUE (engagement_id),
    CONSTRAINT fk_export_settings_engagement
        FOREIGN KEY (organization_id, engagement_id) REFERENCES engagements (organization_id, id) ON DELETE CASCADE
);

CREATE TRIGGER trg_export_settings_set_updated_at
    BEFORE UPDATE ON export_settings FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE TABLE journal_drafts (
    id               BIGSERIAL    PRIMARY KEY,
    uuid             UUID         NOT NULL DEFAULT gen_random_uuid(),
    organization_id  BIGINT       NOT NULL,
    engagement_id    BIGINT       NOT NULL,
    taxpayer_id      BIGINT       NOT NULL REFERENCES taxpayers (id) ON DELETE RESTRICT,
    period           CHAR(7)      NOT NULL,
    entry_no         INT          NOT NULL,
    entry_type       TEXT         NOT NULL DEFAULT 'mahsup',
    entry_date       DATE         NOT NULL,
    description      TEXT         NOT NULL DEFAULT '',
    document_type    TEXT         NULL,
    status           TEXT         NOT NULL DEFAULT 'draft',
    -- Köken: kaynak belgeler, kural, model, güven (G12).
    origin           JSONB        NOT NULL DEFAULT '{}'::jsonb,
    approved_by      BIGINT       NULL REFERENCES users (id) ON DELETE SET NULL,
    approved_at      TIMESTAMPTZ  NULL,
    created_by       BIGINT       NULL REFERENCES users (id) ON DELETE SET NULL,
    created_at       TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at       TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_journal_drafts_uuid UNIQUE (uuid),
    CONSTRAINT uq_journal_drafts_eng_id UNIQUE (engagement_id, id),
    CONSTRAINT uq_journal_drafts_number UNIQUE (engagement_id, period, entry_no),
    CONSTRAINT fk_journal_drafts_engagement
        FOREIGN KEY (organization_id, engagement_id) REFERENCES engagements (organization_id, id) ON DELETE CASCADE,
    CONSTRAINT chk_journal_drafts_entry_type CHECK (entry_type IN ('mahsup', 'acilis', 'kapanis')),
    CONSTRAINT chk_journal_drafts_status CHECK (status IN ('draft', 'approved', 'exported')),
    CONSTRAINT chk_journal_drafts_period CHECK (period ~ '^[0-9]{4}-(0[1-9]|1[0-2])$'),
    CONSTRAINT chk_journal_drafts_entry_no CHECK (entry_no > 0),
    CONSTRAINT chk_journal_drafts_approved CHECK ((status = 'draft') = (approved_at IS NULL))
);

CREATE INDEX idx_journal_drafts_list ON journal_drafts (organization_id, engagement_id, period, status);

CREATE TRIGGER trg_journal_drafts_set_updated_at
    BEFORE UPDATE ON journal_drafts FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE TABLE journal_draft_lines (
    id                BIGSERIAL      PRIMARY KEY,
    organization_id   BIGINT         NOT NULL,
    engagement_id     BIGINT         NOT NULL,
    journal_draft_id  BIGINT         NOT NULL,
    line_no           INT            NOT NULL,
    account_code      TEXT           NOT NULL,
    description       TEXT           NOT NULL DEFAULT '',
    debit             NUMERIC(18,4)  NOT NULL DEFAULT 0,
    credit            NUMERIC(18,4)  NOT NULL DEFAULT 0,
    quantity          NUMERIC(18,6)  NULL,
    currency          CHAR(3)        NULL,
    exchange_rate     NUMERIC(18,6)  NULL,
    foreign_amount    NUMERIC(18,4)  NULL,
    document_no       TEXT           NULL,
    document_date     DATE           NULL,
    -- Kaynak belge (ETTN / banka hareketi / fiş). Çift kayıt kontrolü buna dayanır.
    source_kind       TEXT           NULL,
    source_id         TEXT           NULL,
    confidence        NUMERIC(5,4)   NULL,
    CONSTRAINT uq_journal_draft_lines_line UNIQUE (journal_draft_id, line_no),
    CONSTRAINT fk_journal_draft_lines_draft
        FOREIGN KEY (engagement_id, journal_draft_id) REFERENCES journal_drafts (engagement_id, id) ON DELETE CASCADE,
    CONSTRAINT chk_journal_draft_lines_amounts CHECK (debit >= 0 AND credit >= 0 AND (debit = 0 OR credit = 0)),
    CONSTRAINT chk_journal_draft_lines_account CHECK (btrim(account_code) <> ''),
    CONSTRAINT chk_journal_draft_lines_source CHECK ((source_kind IS NULL) = (source_id IS NULL)),
    CONSTRAINT chk_journal_draft_lines_source_kind CHECK (source_kind IS NULL OR source_kind IN ('invoice', 'bank_transaction', 'receipt', 'manual')),
    CONSTRAINT chk_journal_draft_lines_confidence CHECK (confidence IS NULL OR (confidence >= 0 AND confidence <= 1))
);

CREATE INDEX idx_journal_draft_lines_source ON journal_draft_lines (engagement_id, source_kind, source_id) WHERE source_id IS NOT NULL;

-- Bir çıktı: sürücü + şablon sürümü + kapsam + üretilen dosyalar (SHA-256).
CREATE TABLE export_batches (
    id                BIGSERIAL    PRIMARY KEY,
    uuid              UUID         NOT NULL DEFAULT gen_random_uuid(),
    organization_id   BIGINT       NOT NULL,
    engagement_id     BIGINT       NOT NULL,
    taxpayer_id       BIGINT       NOT NULL REFERENCES taxpayers (id) ON DELETE RESTRICT,
    driver            TEXT         NOT NULL,
    template_version  TEXT         NOT NULL,
    template_verified BOOLEAN      NOT NULL,
    period            CHAR(7)      NULL,
    -- [{name, content_type, sha256, size, entries, storage_key}]
    files             JSONB        NOT NULL DEFAULT '[]'::jsonb,
    issues            JSONB        NOT NULL DEFAULT '[]'::jsonb,
    created_by        BIGINT       NOT NULL REFERENCES users (id) ON DELETE RESTRICT,
    created_at        TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_export_batches_uuid UNIQUE (uuid),
    CONSTRAINT uq_export_batches_eng_id UNIQUE (engagement_id, id),
    CONSTRAINT fk_export_batches_engagement
        FOREIGN KEY (organization_id, engagement_id) REFERENCES engagements (organization_id, id) ON DELETE CASCADE,
    CONSTRAINT chk_export_batches_driver CHECK (driver ~ '^[a-z][a-z0-9_]*$')
);

CREATE INDEX idx_export_batches_list ON export_batches (organization_id, engagement_id, created_at DESC);

-- Fiş ↔ batch. Bir fiş en fazla bir batch'te exported (geri alınmamış) olur.
-- Yeniden çıktı için önce geri alma: kim, ne zaman, neden zorunlu (docs/02 §4).
CREATE TABLE export_batch_items (
    id                BIGSERIAL    PRIMARY KEY,
    organization_id   BIGINT       NOT NULL,
    engagement_id     BIGINT       NOT NULL,
    export_batch_id   BIGINT       NOT NULL,
    journal_draft_id  BIGINT       NOT NULL,
    reversed_at       TIMESTAMPTZ  NULL,
    reversed_by       BIGINT       NULL REFERENCES users (id) ON DELETE RESTRICT,
    reversal_reason   TEXT         NULL,
    created_at        TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    CONSTRAINT fk_export_batch_items_batch
        FOREIGN KEY (engagement_id, export_batch_id) REFERENCES export_batches (engagement_id, id) ON DELETE CASCADE,
    CONSTRAINT fk_export_batch_items_draft
        FOREIGN KEY (engagement_id, journal_draft_id) REFERENCES journal_drafts (engagement_id, id) ON DELETE RESTRICT,
    CONSTRAINT uq_export_batch_items_pair UNIQUE (export_batch_id, journal_draft_id),
    CONSTRAINT chk_export_batch_items_reversal CHECK (
        (reversed_at IS NULL AND reversed_by IS NULL AND reversal_reason IS NULL)
        OR (reversed_at IS NOT NULL AND reversed_by IS NOT NULL AND btrim(COALESCE(reversal_reason, '')) <> ''))
);

CREATE UNIQUE INDEX uq_export_batch_items_live ON export_batch_items (journal_draft_id) WHERE reversed_at IS NULL;

-- RLS (issue #6): engagement kapsamlı tablolar.
SELECT rls_apply_engagement(t) FROM unnest(ARRAY[
    'export_settings', 'journal_drafts', 'journal_draft_lines', 'export_batches', 'export_batch_items'
]::regclass[]) AS t;

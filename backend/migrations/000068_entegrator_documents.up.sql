-- Entegratör belge indeksi (issue #40, 37 §5.1): listeden görülen her belge
-- önce buraya yazılır; UBL indirme kuyruğu, durum güncelleme ve "listede var
-- ama UBL yok" kontrolü bu tablo üzerinden yürür. Mükellefin kaynak verisi
-- (taxpayer_tables.txt).
CREATE TABLE entegrator_documents (
    id                 BIGSERIAL    PRIMARY KEY,
    uuid               UUID         NOT NULL DEFAULT gen_random_uuid(),
    taxpayer_id        BIGINT       NOT NULL REFERENCES taxpayers (id) ON DELETE RESTRICT,
    connection_id      BIGINT       NOT NULL,
    -- ETTN, küçük harf.
    ettn               TEXT         NOT NULL,
    doc_kind           TEXT         NOT NULL,
    direction          TEXT         NOT NULL,
    invoice_number     TEXT         NULL,
    issue_date         DATE         NULL,
    created_at_source  TIMESTAMPTZ  NULL,
    status_raw         TEXT         NOT NULL DEFAULT '',
    status_normalized  TEXT         NOT NULL,
    warnings           TEXT[]       NOT NULL DEFAULT '{}',
    -- Liste yanıtındaki başlık; ortak modele yazılmaz (30 §6).
    header_raw         JSONB        NOT NULL DEFAULT '{}'::jsonb,
    ubl_status         TEXT         NOT NULL DEFAULT 'pending',
    ubl_attempts       INT          NOT NULL DEFAULT 0,
    ubl_error          JSONB        NULL,
    ubl_attachment_id  BIGINT       NULL,
    invoice_id         BIGINT       NULL,
    first_seen_at      TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    last_seen_at       TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    missing_since      TIMESTAMPTZ  NULL,
    created_at         TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at         TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_entegrator_documents_uuid UNIQUE (uuid),
    CONSTRAINT uq_entegrator_documents_taxpayer_id UNIQUE (taxpayer_id, id),
    CONSTRAINT uq_entegrator_documents_key UNIQUE (connection_id, ettn, direction),
    CONSTRAINT fk_entegrator_documents_connection
        FOREIGN KEY (taxpayer_id, connection_id) REFERENCES source_connections (taxpayer_id, id) ON DELETE CASCADE,
    CONSTRAINT fk_entegrator_documents_attachment
        FOREIGN KEY (taxpayer_id, ubl_attachment_id) REFERENCES attachments (taxpayer_id, id) ON DELETE RESTRICT,
    CONSTRAINT fk_entegrator_documents_invoice
        FOREIGN KEY (taxpayer_id, invoice_id) REFERENCES invoices (taxpayer_id, id) ON DELETE RESTRICT,
    CONSTRAINT chk_entegrator_documents_ettn CHECK (ettn <> '' AND ettn = lower(ettn)),
    CONSTRAINT chk_entegrator_documents_doc_kind CHECK (doc_kind IN ('e_invoice', 'e_archive')),
    CONSTRAINT chk_entegrator_documents_direction CHECK (direction IN ('incoming', 'outgoing')),
    CONSTRAINT chk_entegrator_documents_status CHECK (status_normalized IN ('draft', 'active', 'cancelled')),
    CONSTRAINT chk_entegrator_documents_ubl_status CHECK (ubl_status IN ('pending', 'ok', 'failed', 'not_available'))
);

CREATE INDEX idx_entegrator_documents_ubl_queue ON entegrator_documents (connection_id, id)
    WHERE ubl_status IN ('pending', 'failed');
CREATE INDEX idx_entegrator_documents_invoice ON entegrator_documents (taxpayer_id, invoice_id);
CREATE INDEX idx_entegrator_documents_created ON entegrator_documents (connection_id, direction, doc_kind, created_at_source);

CREATE TRIGGER trg_entegrator_documents_set_updated_at
    BEFORE UPDATE ON entegrator_documents FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- RLS (#6, DATABASE_RULES.md): mükellef kapsamı.
SELECT rls_apply_taxpayer('entegrator_documents');

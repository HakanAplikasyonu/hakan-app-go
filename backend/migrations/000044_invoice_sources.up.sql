-- Faturanın kaynakları (issue #12, 01 §6). Aynı fatura birden fazla kaynaktan
-- gelebilir (entegratör, program, UBL yükleme, OCR). invoices satırı tektir ve
-- en yüksek öncelikli kaynağın içeriğini taşır; her kaynak burada bir satırdır.
-- priority: 3 entegratör / UBL XML, 2 ön muhasebe programı (API veya Excel),
-- 1 OCR / elle / diğer.
CREATE TABLE invoice_sources (
    id              BIGSERIAL    PRIMARY KEY,
    taxpayer_id     BIGINT       NOT NULL,
    invoice_id      BIGINT       NOT NULL,
    source_system   TEXT         NOT NULL,
    source_id       TEXT         NOT NULL,
    capture_method  TEXT         NOT NULL,
    priority        SMALLINT     NOT NULL,
    match_rule      TEXT         NOT NULL,
    content_hash    CHAR(64)     NULL,
    attachment_id   BIGINT       NULL,
    raw_payload     JSONB        NOT NULL DEFAULT '{}'::jsonb,
    first_seen_at   TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    last_seen_at    TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_invoice_sources_source UNIQUE (taxpayer_id, source_system, source_id),
    CONSTRAINT fk_invoice_sources_invoice
        FOREIGN KEY (taxpayer_id, invoice_id) REFERENCES invoices (taxpayer_id, id) ON DELETE CASCADE,
    CONSTRAINT fk_invoice_sources_attachment
        FOREIGN KEY (taxpayer_id, attachment_id) REFERENCES attachments (taxpayer_id, id),
    CONSTRAINT chk_invoice_sources_priority CHECK (priority BETWEEN 1 AND 3),
    CONSTRAINT chk_invoice_sources_match_rule CHECK (match_rule IN ('new', 'source', 'ettn', 'number'))
);

CREATE INDEX idx_invoice_sources_invoice ON invoice_sources (invoice_id);

-- Mevcut faturaların kendi kaynağı.
INSERT INTO invoice_sources (taxpayer_id, invoice_id, source_system, source_id, capture_method, priority, match_rule, content_hash, attachment_id, first_seen_at, last_seen_at)
SELECT taxpayer_id, id, source_system, source_id, capture_method,
       CASE
           WHEN source_system LIKE 'entegrator\_%' OR capture_method = 'ubl_upload' THEN 3
           WHEN capture_method IN ('api', 'excel_import') THEN 2
           ELSE 1
       END,
       'new', content_hash, ubl_attachment_id, created_at, updated_at
FROM invoices
ON CONFLICT DO NOTHING;

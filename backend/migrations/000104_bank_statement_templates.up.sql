-- Banka ekstresi şablon kaydı (issue #383). Bankaların "Excel'e aktar" /
-- PDF çıktıları başlık satırından tanınır: parmak izi, sıralı ve normalize
-- edilmiş başlık adlarının SHA-256'sıdır (excelimport ile aynı mantık).
-- Büyük bankaların başlangıç şablonları kodda (statement/templates.go);
-- burada ofisin kaydettikleri durur: bilinmeyen biçimde kolonlar AI ya da
-- sezgiyle önerilir, müşavir onaylayınca şablon olur ve aynı biçimdeki
-- sonraki ekstreler doğrudan okunur. Bir banka biçimi ofisin bütün
-- mükelleflerinde aynıdır: kapsam ofis.
CREATE TABLE bank_statement_templates (
    id               BIGSERIAL    PRIMARY KEY,
    uuid             UUID         NOT NULL DEFAULT gen_random_uuid(),
    organization_id  BIGINT       NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    -- cash_boxes.bank_id ile aynı kimlik (ziraat, isbank ...); NULL bilinmiyor.
    bank_id          TEXT         NULL,
    name             TEXT         NOT NULL DEFAULT '',
    -- excel | csv | pdf
    format           TEXT         NOT NULL,
    fingerprint      CHAR(64)     NOT NULL,
    -- Başlık hücreleri sırayla ve her birinin alanı:
    -- [{"header": "İşlem Tarihi", "field": "date"}, ...]; field "" yok sayılır.
    columns          JSONB        NOT NULL,
    hits             INTEGER      NOT NULL DEFAULT 0,
    last_used_at     TIMESTAMPTZ  NULL,
    created_by       BIGINT       NULL REFERENCES users (id) ON DELETE SET NULL,
    created_at       TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at       TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_bank_statement_templates_uuid UNIQUE (uuid),
    CONSTRAINT uq_bank_statement_templates_fingerprint UNIQUE (organization_id, format, fingerprint),
    CONSTRAINT chk_bank_statement_templates_format CHECK (format IN ('excel', 'csv', 'pdf')),
    CONSTRAINT chk_bank_statement_templates_fingerprint CHECK (fingerprint ~ '^[0-9a-f]{64}$'),
    CONSTRAINT chk_bank_statement_templates_bank CHECK (bank_id IS NULL OR bank_id ~ '^[a-z0-9_]+$'),
    CONSTRAINT chk_bank_statement_templates_columns CHECK (jsonb_typeof(columns) = 'array')
);

CREATE TRIGGER trg_bank_statement_templates_set_updated_at
    BEFORE UPDATE ON bank_statement_templates FOR EACH ROW EXECUTE FUNCTION set_updated_at();

SELECT rls_apply_org('bank_statement_templates');

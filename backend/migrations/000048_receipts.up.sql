-- Fiş yakalama (issue #23, docs/00 K8): masraf / yakıt fişi ve Z raporu
-- fotoğrafından taslak kayıt, mükellef araç listesi, plaka kontrolü.

-- Mükellef araç listesi (T8). Kaynak veri: mükellefe ait.
CREATE TABLE taxpayer_vehicles (
    id          BIGSERIAL    PRIMARY KEY,
    uuid        UUID         NOT NULL DEFAULT gen_random_uuid(),
    taxpayer_id BIGINT       NOT NULL REFERENCES taxpayers (id) ON DELETE CASCADE,
    plate       VARCHAR(16)  NOT NULL,
    description TEXT         NOT NULL DEFAULT '',
    valid_from  DATE         NULL,
    valid_to    DATE         NULL,
    created_by  BIGINT       NULL REFERENCES users (id) ON DELETE SET NULL,
    created_at  TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_taxpayer_vehicles_uuid UNIQUE (uuid),
    CONSTRAINT uq_taxpayer_vehicles_plate UNIQUE (taxpayer_id, plate),
    CONSTRAINT chk_taxpayer_vehicles_plate CHECK (plate ~ '^(0[1-9]|[1-7][0-9]|8[01])[A-Z]{1,3}[0-9]{2,5}$'),
    CONSTRAINT chk_taxpayer_vehicles_range CHECK (valid_to IS NULL OR valid_from IS NULL OR valid_to >= valid_from)
);

CREATE TRIGGER trg_taxpayer_vehicles_set_updated_at
    BEFORE UPDATE ON taxpayer_vehicles FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- expenses (01 §4.12). Bu sürüm yalnızca fiş yakalamanın yazdığı alanları açar.
CREATE TABLE expenses (
    id                   BIGSERIAL      PRIMARY KEY,
    uuid                 UUID           NOT NULL DEFAULT gen_random_uuid(),
    taxpayer_id          BIGINT         NOT NULL REFERENCES taxpayers (id) ON DELETE RESTRICT,
    source_system        TEXT           NOT NULL,
    source_id            TEXT           NOT NULL,
    capture_method       TEXT           NOT NULL,
    expense_date         DATE           NULL,
    period               CHAR(7)        NULL,
    amount               NUMERIC(18,4)  NULL,
    currency             CHAR(3)        NOT NULL DEFAULT 'TRY',
    tax_rate             NUMERIC(5,2)   NULL,
    tax_exclusive_amount NUMERIC(18,4)  NULL,
    tax_amount           NUMERIC(18,4)  NULL,
    description          TEXT           NOT NULL DEFAULT '',
    receipt_type         TEXT           NOT NULL DEFAULT 'other',
    vendor_tax_number    VARCHAR(11)    NULL,
    vendor_name          TEXT           NULL,
    receipt_number       TEXT           NULL,
    z_report_number      TEXT           NULL,
    okc_serial_number    TEXT           NULL,
    vehicle_plate        VARCHAR(16)    NULL,
    payment_method       TEXT           NULL,
    is_deductible        TEXT           NOT NULL DEFAULT 'unknown',
    approval_status      TEXT           NOT NULL DEFAULT 'pending',
    ocr_confidence       NUMERIC(5,4)   NULL,
    ocr_fields           JSONB          NOT NULL DEFAULT '{}'::jsonb,
    attachment_id        BIGINT         NULL,
    raw_payload          JSONB          NOT NULL DEFAULT '{}'::jsonb,
    content_hash         CHAR(64)       NULL,
    approved_by          BIGINT         NULL REFERENCES users (id) ON DELETE SET NULL,
    approved_at          TIMESTAMPTZ    NULL,
    created_at           TIMESTAMPTZ    NOT NULL DEFAULT NOW(),
    updated_at           TIMESTAMPTZ    NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_expenses_uuid UNIQUE (uuid),
    CONSTRAINT uq_expenses_taxpayer_id UNIQUE (taxpayer_id, id),
    CONSTRAINT uq_expenses_source UNIQUE (taxpayer_id, source_system, source_id),
    CONSTRAINT fk_expenses_attachment FOREIGN KEY (taxpayer_id, attachment_id) REFERENCES attachments (taxpayer_id, id),
    CONSTRAINT chk_expenses_capture CHECK (capture_method IN ('api', 'excel_import', 'ocr', 'manual', 'qr')),
    CONSTRAINT chk_expenses_receipt_type CHECK (receipt_type IN (
        'okc_receipt', 'fuel_receipt', 'expense_voucher', 'smm', 'bank_fee', 'salary', 'tax_payment', 'other')),
    CONSTRAINT chk_expenses_payment CHECK (payment_method IS NULL OR payment_method IN (
        'cash', 'bank_transfer', 'credit_card', 'check', 'promissory_note', 'credit')),
    CONSTRAINT chk_expenses_deductible CHECK (is_deductible IN ('unknown', 'yes', 'no')),
    CONSTRAINT chk_expenses_approval CHECK (approval_status IN ('pending', 'approved', 'rejected')),
    CONSTRAINT chk_expenses_period CHECK (period IS NULL OR period ~ '^[0-9]{4}-(0[1-9]|1[0-2])$'),
    CONSTRAINT chk_expenses_amounts CHECK (
        (amount IS NULL OR amount >= 0) AND (tax_amount IS NULL OR tax_amount >= 0)
        AND (tax_exclusive_amount IS NULL OR tax_exclusive_amount >= 0))
);

CREATE INDEX idx_expenses_period ON expenses (taxpayer_id, period);
CREATE INDEX idx_expenses_dup ON expenses (taxpayer_id, vendor_tax_number, expense_date, amount);

CREATE TRIGGER trg_expenses_set_updated_at
    BEFORE UPDATE ON expenses FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- retail_sales (01 §4.7), Z raporu günlük toplamı için en sade hali.
CREATE TABLE retail_sales (
    id                BIGSERIAL      PRIMARY KEY,
    uuid              UUID           NOT NULL DEFAULT gen_random_uuid(),
    taxpayer_id       BIGINT         NOT NULL REFERENCES taxpayers (id) ON DELETE RESTRICT,
    source_system     TEXT           NOT NULL,
    source_id         TEXT           NOT NULL,
    capture_method    TEXT           NOT NULL,
    sale_date         DATE           NULL,
    period            CHAR(7)        NULL,
    z_report_number   TEXT           NULL,
    okc_serial_number TEXT           NULL,
    subtotal_amount   NUMERIC(18,4)  NULL,
    tax_amount        NUMERIC(18,4)  NULL,
    total_amount      NUMERIC(18,4)  NULL,
    currency          CHAR(3)        NOT NULL DEFAULT 'TRY',
    invoice_id        BIGINT         NULL,
    approval_status   TEXT           NOT NULL DEFAULT 'pending',
    ocr_confidence    NUMERIC(5,4)   NULL,
    ocr_fields        JSONB          NOT NULL DEFAULT '{}'::jsonb,
    attachment_id     BIGINT         NULL,
    raw_payload       JSONB          NOT NULL DEFAULT '{}'::jsonb,
    content_hash      CHAR(64)       NULL,
    approved_by       BIGINT         NULL REFERENCES users (id) ON DELETE SET NULL,
    approved_at       TIMESTAMPTZ    NULL,
    created_at        TIMESTAMPTZ    NOT NULL DEFAULT NOW(),
    updated_at        TIMESTAMPTZ    NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_retail_sales_uuid UNIQUE (uuid),
    CONSTRAINT uq_retail_sales_taxpayer_id UNIQUE (taxpayer_id, id),
    CONSTRAINT uq_retail_sales_source UNIQUE (taxpayer_id, source_system, source_id),
    CONSTRAINT fk_retail_sales_attachment FOREIGN KEY (taxpayer_id, attachment_id) REFERENCES attachments (taxpayer_id, id),
    CONSTRAINT fk_retail_sales_invoice FOREIGN KEY (taxpayer_id, invoice_id) REFERENCES invoices (taxpayer_id, id),
    CONSTRAINT chk_retail_sales_capture CHECK (capture_method IN ('api', 'excel_import', 'ocr', 'manual', 'qr')),
    CONSTRAINT chk_retail_sales_approval CHECK (approval_status IN ('pending', 'approved', 'rejected')),
    CONSTRAINT chk_retail_sales_period CHECK (period IS NULL OR period ~ '^[0-9]{4}-(0[1-9]|1[0-2])$')
);

CREATE INDEX idx_retail_sales_period ON retail_sales (taxpayer_id, period);

CREATE TRIGGER trg_retail_sales_set_updated_at
    BEFORE UPDATE ON retail_sales FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- receipt_captures: her yükleme (ofis iş akışı). Okuma sonucu, alan güvenleri,
-- oluşan taslak ve mükerrer tespiti.
CREATE TABLE receipt_captures (
    id              BIGSERIAL    PRIMARY KEY,
    uuid            UUID         NOT NULL DEFAULT gen_random_uuid(),
    organization_id BIGINT       NOT NULL,
    engagement_id   BIGINT       NOT NULL,
    taxpayer_id     BIGINT       NOT NULL REFERENCES taxpayers (id) ON DELETE RESTRICT,
    uploaded_by     BIGINT       NULL REFERENCES users (id) ON DELETE SET NULL,
    attachment_id   BIGINT       NOT NULL,
    media_type      TEXT         NOT NULL,
    kind            TEXT         NULL,
    status          TEXT         NOT NULL,
    fields          JSONB        NOT NULL DEFAULT '{}'::jsonb,
    model           TEXT         NULL,
    expense_id      BIGINT       NULL,
    retail_sale_id  BIGINT       NULL,
    duplicate_of    UUID         NULL,
    error           TEXT         NULL,
    created_at      TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_receipt_captures_uuid UNIQUE (uuid),
    CONSTRAINT fk_receipt_captures_engagement
        FOREIGN KEY (organization_id, engagement_id) REFERENCES engagements (organization_id, id) ON DELETE CASCADE,
    CONSTRAINT fk_receipt_captures_attachment FOREIGN KEY (taxpayer_id, attachment_id) REFERENCES attachments (taxpayer_id, id),
    CONSTRAINT fk_receipt_captures_expense FOREIGN KEY (taxpayer_id, expense_id) REFERENCES expenses (taxpayer_id, id) ON DELETE SET NULL (expense_id),
    CONSTRAINT fk_receipt_captures_retail FOREIGN KEY (taxpayer_id, retail_sale_id) REFERENCES retail_sales (taxpayer_id, id) ON DELETE SET NULL (retail_sale_id),
    CONSTRAINT chk_receipt_captures_kind CHECK (kind IS NULL OR kind IN ('expense_receipt', 'fuel_receipt', 'z_report', 'other')),
    CONSTRAINT chk_receipt_captures_status CHECK (status IN ('draft', 'duplicate', 'failed', 'approved', 'rejected'))
);

CREATE INDEX idx_receipt_captures_list ON receipt_captures (organization_id, engagement_id, created_at DESC);

CREATE TRIGGER trg_receipt_captures_set_updated_at
    BEFORE UPDATE ON receipt_captures FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- RLS (issue #6).
SELECT rls_apply_taxpayer(t) FROM unnest(ARRAY['taxpayer_vehicles', 'expenses', 'retail_sales']::regclass[]) AS t;
SELECT rls_apply_engagement('receipt_captures'::regclass);

INSERT INTO permissions (name, slug) VALUES
    ('Read tenant receipts', 'tenant.receipts.read'),
    ('Upload tenant receipts', 'tenant.receipts.write'),
    ('Approve tenant receipts', 'tenant.receipts.approve')
ON CONFLICT (slug) DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM roles r
JOIN permissions p ON p.slug IN ('tenant.receipts.read', 'tenant.receipts.write', 'tenant.receipts.approve')
WHERE r.slug IN ('owner', 'admin') AND r.scope = 'tenant' AND r.organization_id IS NULL
ON CONFLICT DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM roles r
JOIN permissions p ON p.slug IN ('tenant.receipts.read', 'tenant.receipts.write')
WHERE r.slug = 'member' AND r.scope = 'tenant' AND r.organization_id IS NULL
ON CONFLICT DO NOTHING;

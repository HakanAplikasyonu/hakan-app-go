-- Ortak veri modeli (issue #9). Kaynak: docs/kaynak/musavir-entegrasyon-paketi/01-ortak-veri-modeli.md
--
-- İki kapsam (docs/01 §2.3):
--   * Kaynak veri mükellefindir: taxpayer_id NOT NULL. Bağlantılar, cariler,
--     faturalar, ekler, senkron durumu. Ofis değişince veri mükellefte kalır (D2).
--     Lint: internal/database/taxpayer_tables.txt.
--   * Ofisin iş ürünü: organization_id + engagement_id NOT NULL. Uyarı kararları
--     ve kilitli döneme gelen değişiklik talepleri. Lint: tenant_tables.txt.
--
-- Paketteki company_id burada taxpayer_id'dir. Boilerplate kuralı gereği iç
-- anahtar BIGSERIAL id, dışa açılan kimlik uuid'dir; ETTN bu yüzden invoices.ettn.
-- Tutarlar NUMERIC(18,4), oranlar NUMERIC(5,2), miktar/birim fiyat/kur NUMERIC(18,6).
-- Enum'lar TEXT + CHECK.

-- ---------------------------------------------------------------------------
-- source_connections (01 §4.2)
-- ---------------------------------------------------------------------------
CREATE TABLE source_connections (
    id                  BIGSERIAL    PRIMARY KEY,
    uuid                UUID         NOT NULL DEFAULT gen_random_uuid(),
    taxpayer_id         BIGINT       NOT NULL REFERENCES taxpayers (id) ON DELETE RESTRICT,
    source_system       TEXT         NOT NULL,
    external_company_id TEXT         NULL,
    external_branch_ids JSONB        NOT NULL DEFAULT '[]'::jsonb,
    -- Sır kasası referansı (internal/platform/secrets, "vault:<uuid>"). Kimlik
    -- bilgisi bu tabloda hiçbir zaman açık tutulmaz.
    credentials_ref     TEXT         NULL,
    setup_state         TEXT         NOT NULL DEFAULT 'draft',
    status              TEXT         NOT NULL DEFAULT 'pending',
    last_error          JSONB        NULL,
    meta                JSONB        NOT NULL DEFAULT '{}'::jsonb,
    consent_text        TEXT         NULL,
    consent_at          TIMESTAMPTZ  NULL,
    sync_cursor         JSONB        NOT NULL DEFAULT '{}'::jsonb,
    capabilities        JSONB        NOT NULL DEFAULT '{}'::jsonb,
    last_full_sync_at   TIMESTAMPTZ  NULL,
    last_success_at     TIMESTAMPTZ  NULL,
    created_at          TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at          TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_source_connections_uuid UNIQUE (uuid),
    CONSTRAINT uq_source_connections_taxpayer_id UNIQUE (taxpayer_id, id),
    CONSTRAINT chk_source_connections_source_system CHECK (source_system ~ '^(easytrade|parasut|kolaybi|bizimhesap|logo_isbasi|logo_tiger|logo_go|mikro|robom|manual_upload|entegrator_[a-z0-9_]+|bank_[a-z0-9_]+)$'),
    CONSTRAINT chk_source_connections_credentials_ref CHECK (credentials_ref IS NULL OR credentials_ref ~ '^[a-z]+:'),
    CONSTRAINT chk_source_connections_setup_state CHECK (setup_state IN (
        'draft', 'credentials_entered', 'authorization_requested', 'form_ready',
        'submitted_to_bank', 'verifying', 'active', 'failed')),
    CONSTRAINT chk_source_connections_status CHECK (status IN ('pending', 'active', 'auth_failed', 'rate_limited', 'disabled'))
);

CREATE INDEX idx_source_connections_taxpayer ON source_connections (taxpayer_id, source_system);

CREATE TRIGGER trg_source_connections_set_updated_at
    BEFORE UPDATE ON source_connections FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- ---------------------------------------------------------------------------
-- customers (cari, 01 §4.3). Mükellefin kendi cari kaydı; başka bir mükellefe
-- bağlanmaz (D1b). Aynı VKN / TCKN tek kayıtta birleşir, kaynaklar
-- customer_source_links'te.
-- ---------------------------------------------------------------------------
CREATE TABLE customers (
    id                          BIGSERIAL    PRIMARY KEY,
    uuid                        UUID         NOT NULL DEFAULT gen_random_uuid(),
    taxpayer_id                 BIGINT       NOT NULL REFERENCES taxpayers (id) ON DELETE RESTRICT,
    source_system               TEXT         NOT NULL,
    source_id                   TEXT         NOT NULL,
    capture_method              TEXT         NOT NULL,
    type                        TEXT         NOT NULL DEFAULT 'both',
    customer_legal_type         TEXT         NULL,
    trade_name                  TEXT         NULL,
    first_name                  TEXT         NULL,
    last_name                   TEXT         NULL,
    name                        TEXT         NOT NULL DEFAULT '',
    tax_number                  VARCHAR(10)  NULL,
    identity_number             VARCHAR(11)  NULL,
    tax_office                  TEXT         NULL,
    address                     TEXT         NULL,
    city                        TEXT         NULL,
    district                    TEXT         NULL,
    postal_code                 TEXT         NULL,
    country                     TEXT         NULL,
    email                       TEXT         NULL,
    phone                       TEXT         NULL,
    is_default_retail_customer  BOOLEAN      NOT NULL DEFAULT FALSE,
    is_abroad                   BOOLEAN      NOT NULL DEFAULT FALSE,
    bank_accounts               JSONB        NOT NULL DEFAULT '[]'::jsonb,
    raw_payload                 JSONB        NOT NULL DEFAULT '{}'::jsonb,
    content_hash                CHAR(64)     NULL,
    deleted_at_source           TIMESTAMPTZ  NULL,
    created_at                  TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at                  TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_customers_uuid UNIQUE (uuid),
    CONSTRAINT uq_customers_taxpayer_id UNIQUE (taxpayer_id, id),
    CONSTRAINT chk_customers_type CHECK (type IN ('customer', 'supplier', 'both', 'employee')),
    CONSTRAINT chk_customers_legal_type CHECK (customer_legal_type IS NULL OR customer_legal_type IN ('individual', 'corporate')),
    CONSTRAINT chk_customers_capture_method CHECK (capture_method IN ('api', 'excel_import', 'ocr', 'manual', 'qr', 'bank_match', 'ubl_upload')),
    CONSTRAINT chk_customers_tax_number CHECK (tax_number IS NULL OR tax_number ~ '^[0-9]{10}$'),
    CONSTRAINT chk_customers_identity_number CHECK (identity_number IS NULL OR identity_number ~ '^[0-9]{11}$')
);

CREATE UNIQUE INDEX uq_customers_taxpayer_tax_number
    ON customers (taxpayer_id, tax_number) WHERE tax_number IS NOT NULL;
CREATE UNIQUE INDEX uq_customers_taxpayer_identity_number
    ON customers (taxpayer_id, identity_number) WHERE identity_number IS NOT NULL AND tax_number IS NULL;
CREATE INDEX idx_customers_taxpayer_name ON customers (taxpayer_id, name);

CREATE TRIGGER trg_customers_set_updated_at
    BEFORE UPDATE ON customers FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE TABLE customer_source_links (
    id            BIGSERIAL    PRIMARY KEY,
    taxpayer_id   BIGINT       NOT NULL,
    customer_id   BIGINT       NOT NULL,
    source_system TEXT         NOT NULL,
    source_id     TEXT         NOT NULL,
    raw_payload   JSONB        NOT NULL DEFAULT '{}'::jsonb,
    created_at    TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at    TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_customer_source_links_source UNIQUE (taxpayer_id, source_system, source_id),
    CONSTRAINT fk_customer_source_links_customer
        FOREIGN KEY (taxpayer_id, customer_id) REFERENCES customers (taxpayer_id, id) ON DELETE CASCADE
);

CREATE INDEX idx_customer_source_links_customer ON customer_source_links (customer_id);

CREATE TRIGGER trg_customer_source_links_set_updated_at
    BEFORE UPDATE ON customer_source_links FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- ---------------------------------------------------------------------------
-- attachments (01 §4.16). Nesne deposunda, mükellef başına SHA-256 ile tekil.
-- ---------------------------------------------------------------------------
CREATE TABLE attachments (
    id           BIGSERIAL    PRIMARY KEY,
    uuid         UUID         NOT NULL DEFAULT gen_random_uuid(),
    taxpayer_id  BIGINT       NOT NULL REFERENCES taxpayers (id) ON DELETE RESTRICT,
    owner_type   TEXT         NULL,
    owner_id     BIGINT       NULL,
    kind         TEXT         NOT NULL,
    storage_key  TEXT         NOT NULL,
    source_url   TEXT         NULL,
    sha256       CHAR(64)     NOT NULL,
    size_bytes   BIGINT       NOT NULL,
    content_type TEXT         NULL,
    file_name    TEXT         NULL,
    created_at   TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at   TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_attachments_uuid UNIQUE (uuid),
    CONSTRAINT uq_attachments_taxpayer_id UNIQUE (taxpayer_id, id),
    CONSTRAINT uq_attachments_taxpayer_sha256 UNIQUE (taxpayer_id, sha256),
    CONSTRAINT chk_attachments_owner CHECK ((owner_type IS NULL) = (owner_id IS NULL)),
    CONSTRAINT chk_attachments_owner_type CHECK (owner_type IS NULL OR owner_type IN ('invoice', 'expense', 'retail_sale')),
    CONSTRAINT chk_attachments_kind CHECK (kind IN ('pdf', 'ubl_xml', 'image', 'html', 'source_file')),
    CONSTRAINT chk_attachments_sha256 CHECK (sha256 ~ '^[0-9a-f]{64}$'),
    CONSTRAINT chk_attachments_size CHECK (size_bytes >= 0)
);

CREATE INDEX idx_attachments_owner ON attachments (taxpayer_id, owner_type, owner_id);

CREATE TRIGGER trg_attachments_set_updated_at
    BEFORE UPDATE ON attachments FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- ---------------------------------------------------------------------------
-- invoices (01 §4.4)
-- ---------------------------------------------------------------------------
CREATE TABLE invoices (
    id                              BIGSERIAL      PRIMARY KEY,
    uuid                            UUID           NOT NULL DEFAULT gen_random_uuid(),
    taxpayer_id                     BIGINT         NOT NULL REFERENCES taxpayers (id) ON DELETE RESTRICT,
    source_system                   TEXT           NOT NULL,
    source_id                       TEXT           NOT NULL,
    capture_method                  TEXT           NOT NULL,
    -- ETTN (paketteki invoice.uuid). Mükerrer kontrolünün birinci anahtarı;
    -- tekil değil (01 §6).
    ettn                            UUID           NULL,
    invoice_number                  VARCHAR(32)    NOT NULL,
    direction                       TEXT           NOT NULL,
    type                            TEXT           NOT NULL,
    invoice_type_code_raw           TEXT           NULL,
    profile                         TEXT           NULL,
    profile_raw                     TEXT           NULL,
    document_class                  TEXT           NOT NULL DEFAULT 'invoice',
    status                          TEXT           NULL,
    status_normalized               TEXT           NOT NULL DEFAULT 'active',
    issue_date                      DATE           NOT NULL,
    issue_time                      TEXT           NULL,
    payment_due_date                DATE           NULL,
    period                          CHAR(7)        NOT NULL,
    customer_id                     BIGINT         NULL,
    customer_tax_number             VARCHAR(10)    NULL,
    customer_identity_number        VARCHAR(11)    NULL,
    customer_name                   TEXT           NULL,
    currency                        CHAR(3)        NOT NULL,
    exchange_rate                   NUMERIC(18,6)  NULL,
    line_extension_amount           NUMERIC(18,4)  NOT NULL DEFAULT 0,
    allowance_total_amount          NUMERIC(18,4)  NOT NULL DEFAULT 0,
    tax_exclusive_amount            NUMERIC(18,4)  NOT NULL DEFAULT 0,
    tax_amount                      NUMERIC(18,4)  NOT NULL DEFAULT 0,
    tax_inclusive_amount            NUMERIC(18,4)  NOT NULL DEFAULT 0,
    withholding_amount              NUMERIC(18,4)  NOT NULL DEFAULT 0,
    excise_amount                   NUMERIC(18,4)  NOT NULL DEFAULT 0,
    communications_tax_amount       NUMERIC(18,4)  NOT NULL DEFAULT 0,
    income_tax_withholding_amount   NUMERIC(18,4)  NOT NULL DEFAULT 0,
    income_tax_withholding_rate     NUMERIC(5,2)   NULL,
    payable_amount                  NUMERIC(18,4)  NOT NULL DEFAULT 0,
    payable_amount_try              NUMERIC(18,4)  NULL,
    global_tax_exemption_code       VARCHAR(3)     NULL,
    original_invoice_number         VARCHAR(32)    NULL,
    original_invoice_issue_date     DATE           NULL,
    order_reference                 JSONB          NULL,
    is_internet_sale                BOOLEAN        NULL,
    sgk                             JSONB          NULL,
    despatch_id                     TEXT           NULL,
    notes                           TEXT           NULL,
    payment_status                  TEXT           NOT NULL DEFAULT 'unknown',
    remaining_amount                NUMERIC(18,4)  NULL,
    has_items                       BOOLEAN        NOT NULL DEFAULT FALSE,
    tax_summary_source              TEXT           NOT NULL,
    ubl_attachment_id               BIGINT         NULL,
    raw_payload                     JSONB          NOT NULL DEFAULT '{}'::jsonb,
    content_hash                    CHAR(64)       NULL,
    source_updated_at               TIMESTAMPTZ    NULL,
    deleted_at_source               TIMESTAMPTZ    NULL,
    created_at                      TIMESTAMPTZ    NOT NULL DEFAULT NOW(),
    updated_at                      TIMESTAMPTZ    NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_invoices_uuid UNIQUE (uuid),
    CONSTRAINT uq_invoices_taxpayer_id UNIQUE (taxpayer_id, id),
    CONSTRAINT uq_invoices_source UNIQUE (taxpayer_id, source_system, source_id),
    CONSTRAINT fk_invoices_customer
        FOREIGN KEY (taxpayer_id, customer_id) REFERENCES customers (taxpayer_id, id),
    CONSTRAINT fk_invoices_ubl_attachment
        FOREIGN KEY (taxpayer_id, ubl_attachment_id) REFERENCES attachments (taxpayer_id, id),
    CONSTRAINT chk_invoices_capture_method CHECK (capture_method IN ('api', 'excel_import', 'ocr', 'manual', 'qr', 'bank_match', 'ubl_upload')),
    CONSTRAINT chk_invoices_direction CHECK (direction IN ('outgoing', 'incoming')),
    CONSTRAINT chk_invoices_type CHECK (type IN ('SATIS', 'ALIS', 'IADE', 'TEVKIFAT', 'ISTISNA', 'OZELMATRAH', 'IHRACKAYITLI', 'SGK')),
    CONSTRAINT chk_invoices_document_class CHECK (document_class IN ('invoice', 'smm', 'mustahsil', 'price_difference')),
    CONSTRAINT chk_invoices_status_normalized CHECK (status_normalized IN ('draft', 'active', 'cancelled')),
    CONSTRAINT chk_invoices_period CHECK (period ~ '^[0-9]{4}-(0[1-9]|1[0-2])$'),
    CONSTRAINT chk_invoices_currency CHECK (currency ~ '^[A-Z]{3}$'),
    CONSTRAINT chk_invoices_payment_status CHECK (payment_status IN ('paid', 'partially_paid', 'unpaid', 'overdue', 'unknown')),
    CONSTRAINT chk_invoices_tax_summary_source CHECK (tax_summary_source IN ('items', 'source_summary', 'header_estimate')),
    CONSTRAINT chk_invoices_amounts_positive CHECK (
        line_extension_amount >= 0 AND allowance_total_amount >= 0 AND tax_exclusive_amount >= 0 AND
        tax_amount >= 0 AND tax_inclusive_amount >= 0 AND withholding_amount >= 0 AND payable_amount >= 0)
);

-- Mükerrer kontrolü (01 §6): ETTN, sonra VKN/TCKN + fatura no + tarih, sonra
-- karşı taraf + tarih + tutar.
CREATE INDEX idx_invoices_taxpayer_ettn ON invoices (taxpayer_id, ettn) WHERE ettn IS NOT NULL;
CREATE INDEX idx_invoices_taxpayer_number ON invoices (taxpayer_id, invoice_number, issue_date);
CREATE INDEX idx_invoices_taxpayer_party_date ON invoices (taxpayer_id, customer_tax_number, customer_identity_number, issue_date);
CREATE INDEX idx_invoices_taxpayer_period ON invoices (taxpayer_id, period, direction);
CREATE INDEX idx_invoices_customer ON invoices (customer_id) WHERE customer_id IS NOT NULL;

CREATE TRIGGER trg_invoices_set_updated_at
    BEFORE UPDATE ON invoices FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- ---------------------------------------------------------------------------
-- invoice_items (01 §4.5)
-- ---------------------------------------------------------------------------
CREATE TABLE invoice_items (
    id                              BIGSERIAL      PRIMARY KEY,
    taxpayer_id                     BIGINT         NOT NULL,
    invoice_id                      BIGINT         NOT NULL,
    line_no                         INT            NOT NULL,
    name                            TEXT           NOT NULL DEFAULT '',
    description                     TEXT           NULL,
    product_code                    TEXT           NULL,
    quantity                        NUMERIC(18,6)  NOT NULL DEFAULT 0,
    unit_code                       VARCHAR(8)     NULL,
    unit_price                      NUMERIC(18,6)  NOT NULL DEFAULT 0,
    line_extension_amount           NUMERIC(18,4)  NOT NULL DEFAULT 0,
    own_allowance_amount            NUMERIC(18,4)  NOT NULL DEFAULT 0,
    allowance_total_amount          NUMERIC(18,4)  NOT NULL DEFAULT 0,
    tax_exclusive_amount            NUMERIC(18,4)  NOT NULL DEFAULT 0,
    tax_rate                        NUMERIC(5,2)   NOT NULL DEFAULT 0,
    total_tax_amount                NUMERIC(18,4)  NOT NULL DEFAULT 0,
    tax_inclusive_amount            NUMERIC(18,4)  NOT NULL DEFAULT 0,
    withholding_code                VARCHAR(3)     NULL,
    withholding_rate                NUMERIC(5,2)   NOT NULL DEFAULT 0,
    withholding_amount              NUMERIC(18,4)  NOT NULL DEFAULT 0,
    tax_exemption_code              VARCHAR(3)     NULL,
    special_tax_base_code           VARCHAR(3)     NULL,
    special_tax_base_amount         NUMERIC(18,4)  NULL,
    export_registered_code          VARCHAR(3)     NULL,
    gtip_code                       VARCHAR(12)    NULL,
    diib_line_code                  TEXT           NULL,
    excise_rate                     NUMERIC(5,2)   NOT NULL DEFAULT 0,
    excise_amount                   NUMERIC(18,4)  NOT NULL DEFAULT 0,
    communications_tax_rate         NUMERIC(5,2)   NOT NULL DEFAULT 0,
    communications_tax_amount       NUMERIC(18,4)  NOT NULL DEFAULT 0,
    income_tax_withholding_amount   NUMERIC(18,4)  NOT NULL DEFAULT 0,
    expense_category_id             BIGINT         NULL,
    line_type                       TEXT           NOT NULL DEFAULT 'unknown',
    tax_breakdowns                  JSONB          NOT NULL DEFAULT '[]'::jsonb,
    CONSTRAINT uq_invoice_items_line UNIQUE (invoice_id, line_no),
    CONSTRAINT fk_invoice_items_invoice
        FOREIGN KEY (taxpayer_id, invoice_id) REFERENCES invoices (taxpayer_id, id) ON DELETE CASCADE,
    CONSTRAINT chk_invoice_items_line_type CHECK (line_type IN ('goods', 'service', 'fixed_asset_candidate', 'discount', 'expense', 'unknown'))
);

-- ---------------------------------------------------------------------------
-- invoice_tax_summaries (01 §4.6). KDV1'in beslendiği tablo.
-- ---------------------------------------------------------------------------
CREATE TABLE invoice_tax_summaries (
    id                  BIGSERIAL      PRIMARY KEY,
    taxpayer_id         BIGINT         NOT NULL,
    invoice_id          BIGINT         NOT NULL,
    tax_rate            NUMERIC(5,2)   NOT NULL,
    base                NUMERIC(18,4)  NOT NULL DEFAULT 0,
    vat                 NUMERIC(18,4)  NOT NULL DEFAULT 0,
    withholding         NUMERIC(18,4)  NOT NULL DEFAULT 0,
    withholding_code    VARCHAR(3)     NULL,
    tax_exemption_code  VARCHAR(3)     NULL,
    CONSTRAINT fk_invoice_tax_summaries_invoice
        FOREIGN KEY (taxpayer_id, invoice_id) REFERENCES invoices (taxpayer_id, id) ON DELETE CASCADE
);

CREATE UNIQUE INDEX uq_invoice_tax_summaries_group
    ON invoice_tax_summaries (invoice_id, tax_rate, COALESCE(withholding_code, ''), COALESCE(tax_exemption_code, ''));

-- ---------------------------------------------------------------------------
-- sync_windows, sync_runs (01 §4.17, 02 §3.1)
-- ---------------------------------------------------------------------------
CREATE TABLE sync_windows (
    id             BIGSERIAL    PRIMARY KEY,
    taxpayer_id    BIGINT       NOT NULL,
    connection_id  BIGINT       NOT NULL,
    item           TEXT         NOT NULL,
    direction      TEXT         NULL,
    date_field     TEXT         NOT NULL DEFAULT 'issue',
    range_start    TIMESTAMPTZ  NOT NULL,
    range_end      TIMESTAMPTZ  NOT NULL,
    status         TEXT         NOT NULL DEFAULT 'pending',
    page_cursor    TEXT         NULL,
    attempts       INT          NOT NULL DEFAULT 0,
    last_error     JSONB        NULL,
    started_at     TIMESTAMPTZ  NULL,
    finished_at    TIMESTAMPTZ  NULL,
    created_at     TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at     TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    CONSTRAINT fk_sync_windows_connection
        FOREIGN KEY (taxpayer_id, connection_id) REFERENCES source_connections (taxpayer_id, id) ON DELETE CASCADE,
    CONSTRAINT uq_sync_windows_range UNIQUE (connection_id, item, date_field, range_start, range_end),
    CONSTRAINT chk_sync_windows_status CHECK (status IN ('pending', 'running', 'done', 'failed')),
    CONSTRAINT chk_sync_windows_range CHECK (range_end > range_start)
);

CREATE INDEX idx_sync_windows_pending ON sync_windows (connection_id, status) WHERE status IN ('pending', 'failed');

CREATE TRIGGER trg_sync_windows_set_updated_at
    BEFORE UPDATE ON sync_windows FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE TABLE sync_runs (
    id             BIGSERIAL    PRIMARY KEY,
    uuid           UUID         NOT NULL DEFAULT gen_random_uuid(),
    taxpayer_id    BIGINT       NOT NULL,
    connection_id  BIGINT       NOT NULL,
    kind           TEXT         NOT NULL,
    status         TEXT         NOT NULL DEFAULT 'running',
    counters       JSONB        NOT NULL DEFAULT '{}'::jsonb,
    error          JSONB        NULL,
    started_at     TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    finished_at    TIMESTAMPTZ  NULL,
    CONSTRAINT uq_sync_runs_uuid UNIQUE (uuid),
    CONSTRAINT fk_sync_runs_connection
        FOREIGN KEY (taxpayer_id, connection_id) REFERENCES source_connections (taxpayer_id, id) ON DELETE CASCADE,
    CONSTRAINT chk_sync_runs_kind CHECK (kind IN ('backfill', 'incremental', 'full_scan', 'status_refresh', 'manual')),
    CONSTRAINT chk_sync_runs_status CHECK (status IN ('running', 'success', 'partial', 'failed'))
);

CREATE INDEX idx_sync_runs_connection ON sync_runs (connection_id, started_at DESC);

-- ---------------------------------------------------------------------------
-- Ofis iş ürünü: data_warnings (05 §1), change_requests (01 §4.17, 02 §3.4).
-- organization_id + engagement_id; engagement'ın mükellefi taxpayer_id'de
-- tekrar tutulur (kaynak kayda bağlanmak için).
-- ---------------------------------------------------------------------------
CREATE TABLE data_warnings (
    id               BIGSERIAL    PRIMARY KEY,
    uuid             UUID         NOT NULL DEFAULT gen_random_uuid(),
    organization_id  BIGINT       NOT NULL,
    engagement_id    BIGINT       NOT NULL,
    taxpayer_id      BIGINT       NOT NULL REFERENCES taxpayers (id) ON DELETE RESTRICT,
    period           CHAR(7)      NULL,
    entity_type      TEXT         NOT NULL,
    entity_id        BIGINT       NULL,
    code             TEXT         NOT NULL,
    severity         TEXT         NOT NULL,
    params           JSONB        NOT NULL DEFAULT '{}'::jsonb,
    producer         TEXT         NOT NULL,
    status           TEXT         NOT NULL DEFAULT 'open',
    resolved_by      BIGINT       NULL REFERENCES users (id) ON DELETE SET NULL,
    resolved_at      TIMESTAMPTZ  NULL,
    created_at       TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at       TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_data_warnings_uuid UNIQUE (uuid),
    CONSTRAINT fk_data_warnings_engagement
        FOREIGN KEY (organization_id, engagement_id) REFERENCES engagements (organization_id, id) ON DELETE CASCADE,
    CONSTRAINT chk_data_warnings_entity_type CHECK (entity_type IN (
        'invoice', 'invoice_item', 'customer', 'expense', 'financial_transaction', 'retail_sale',
        'bank_transaction', 'entegrator_document', 'source_connection', 'period')),
    CONSTRAINT chk_data_warnings_code CHECK (code ~ '^[A-Z][A-Z0-9_]*$'),
    CONSTRAINT chk_data_warnings_severity CHECK (severity IN ('info', 'warning', 'blocker')),
    CONSTRAINT chk_data_warnings_status CHECK (status IN ('open', 'acknowledged', 'resolved', 'auto_resolved')),
    CONSTRAINT chk_data_warnings_period CHECK (period IS NULL OR period ~ '^[0-9]{4}-(0[1-9]|1[0-2])$')
);

-- 05 §1: aynı (varlık, kod) için en fazla bir açık uyarı.
CREATE UNIQUE INDEX uq_data_warnings_open
    ON data_warnings (engagement_id, entity_type, COALESCE(entity_id, 0), code, COALESCE(period, ''))
    WHERE status = 'open';
CREATE INDEX idx_data_warnings_list ON data_warnings (organization_id, engagement_id, status, period);

CREATE TRIGGER trg_data_warnings_set_updated_at
    BEFORE UPDATE ON data_warnings FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE TABLE change_requests (
    id               BIGSERIAL    PRIMARY KEY,
    uuid             UUID         NOT NULL DEFAULT gen_random_uuid(),
    organization_id  BIGINT       NOT NULL,
    engagement_id    BIGINT       NOT NULL,
    taxpayer_id      BIGINT       NOT NULL REFERENCES taxpayers (id) ON DELETE RESTRICT,
    period           CHAR(7)      NOT NULL,
    entity_type      TEXT         NOT NULL,
    entity_id        BIGINT       NULL,
    old_hash         CHAR(64)     NULL,
    new_hash         CHAR(64)     NULL,
    diff             JSONB        NOT NULL DEFAULT '{}'::jsonb,
    new_payload      JSONB        NOT NULL DEFAULT '{}'::jsonb,
    status           TEXT         NOT NULL DEFAULT 'open',
    decided_by       BIGINT       NULL REFERENCES users (id) ON DELETE SET NULL,
    decided_at       TIMESTAMPTZ  NULL,
    created_at       TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at       TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_change_requests_uuid UNIQUE (uuid),
    CONSTRAINT fk_change_requests_engagement
        FOREIGN KEY (organization_id, engagement_id) REFERENCES engagements (organization_id, id) ON DELETE CASCADE,
    CONSTRAINT chk_change_requests_status CHECK (status IN ('open', 'applied', 'rejected')),
    CONSTRAINT chk_change_requests_decided CHECK ((status = 'open') = (decided_at IS NULL)),
    CONSTRAINT chk_change_requests_period CHECK (period ~ '^[0-9]{4}-(0[1-9]|1[0-2])$')
);

CREATE INDEX idx_change_requests_list ON change_requests (organization_id, engagement_id, status, period);

CREATE TRIGGER trg_change_requests_set_updated_at
    BEFORE UPDATE ON change_requests FOR EACH ROW EXECUTE FUNCTION set_updated_at();

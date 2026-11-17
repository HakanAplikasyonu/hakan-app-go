-- Personel, bordro ve SGK primleri (#382).
--
-- Bordro mükellefin (işverenin) kaynak verisidir: banka hareketleri gibi
-- taxpayer_id kapsamlıdır (rls_apply_taxpayer), mükellef başka ofise geçse
-- de veri mükellefte kalır. Ofis yalnızca görünür engagement'larının
-- mükelleflerini görür; mükellef paneli kendi satırlarını görür, ama API
-- panelde yalnızca bordro belgelerini (PDF) açar.
--
-- TCKN düz saklanmaz: ekranda maskeli hâli (national_id_masked) ve aynı
-- kişiyi tekrar tanımak için anahtarlı özeti (national_id_digest, HMAC-SHA256)
-- tutulur.

INSERT INTO permissions (name, slug) VALUES
    ('Read tenant payroll', 'tenant.payroll.read'),
    ('Write tenant payroll', 'tenant.payroll.write')
ON CONFLICT (slug) DO NOTHING;

-- Maaş verisi hassas: stajyer görmez (sahip kendiliğinden alır, 000044).
SELECT grant_tenant_role_permission(r, p)
FROM unnest(ARRAY['admin', 'accountant', 'member']) AS r,
     unnest(ARRAY['tenant.payroll.read', 'tenant.payroll.write']) AS p;

CREATE TABLE employees (
    id                  BIGSERIAL    PRIMARY KEY,
    uuid                UUID         NOT NULL DEFAULT gen_random_uuid(),
    taxpayer_id         BIGINT       NOT NULL REFERENCES taxpayers (id) ON DELETE RESTRICT,
    full_name           TEXT         NOT NULL,
    national_id_masked  VARCHAR(11)  NULL,
    national_id_digest  CHAR(64)     NULL,
    sgk_no              VARCHAR(32)  NULL,
    job_title           TEXT         NULL,
    department          TEXT         NULL,
    iban                VARCHAR(34)  NULL,
    hire_date           DATE         NULL,
    exit_date           DATE         NULL,
    exit_reason         TEXT         NULL,
    created_by          BIGINT       NULL REFERENCES users (id) ON DELETE SET NULL,
    created_at          TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at          TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_employees_uuid UNIQUE (uuid),
    CONSTRAINT uq_employees_taxpayer_id UNIQUE (taxpayer_id, id),
    CONSTRAINT chk_employees_name CHECK (btrim(full_name) <> ''),
    CONSTRAINT chk_employees_masked CHECK (national_id_masked IS NULL OR national_id_masked ~ '^[0-9]{3}\*{6}[0-9]{2}$'),
    CONSTRAINT chk_employees_digest CHECK (national_id_digest IS NULL OR national_id_digest ~ '^[0-9a-f]{64}$'),
    CONSTRAINT chk_employees_national_id CHECK ((national_id_masked IS NULL) = (national_id_digest IS NULL)),
    CONSTRAINT chk_employees_dates CHECK (exit_date IS NULL OR hire_date IS NULL OR exit_date >= hire_date)
);

CREATE UNIQUE INDEX uq_employees_national_id ON employees (taxpayer_id, national_id_digest) WHERE national_id_digest IS NOT NULL;
CREATE INDEX idx_employees_list ON employees (taxpayer_id, full_name);

CREATE TRIGGER trg_employees_set_updated_at
    BEFORE UPDATE ON employees FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- Bir mükellefin bir dönemi için tek bordro. Yeniden içe aktarma satırları
-- değiştirir (fiş taslağı onaylanmadıysa).
CREATE TABLE payroll_runs (
    id                           BIGSERIAL      PRIMARY KEY,
    uuid                         UUID           NOT NULL DEFAULT gen_random_uuid(),
    taxpayer_id                  BIGINT         NOT NULL REFERENCES taxpayers (id) ON DELETE RESTRICT,
    period                       CHAR(7)        NOT NULL,
    source                       TEXT           NOT NULL DEFAULT 'import',
    source_system                TEXT           NOT NULL DEFAULT 'generic',
    source_file_name             TEXT           NULL,
    employee_count               INT            NOT NULL DEFAULT 0,
    gross_total                  NUMERIC(18,2)  NOT NULL DEFAULT 0,
    net_total                    NUMERIC(18,2)  NOT NULL DEFAULT 0,
    sgk_employee_total           NUMERIC(18,2)  NOT NULL DEFAULT 0,
    unemployment_employee_total  NUMERIC(18,2)  NOT NULL DEFAULT 0,
    sgk_employer_total           NUMERIC(18,2)  NOT NULL DEFAULT 0,
    unemployment_employer_total  NUMERIC(18,2)  NOT NULL DEFAULT 0,
    sgk_incentive_total          NUMERIC(18,2)  NOT NULL DEFAULT 0,
    income_tax_total             NUMERIC(18,2)  NOT NULL DEFAULT 0,
    stamp_tax_total              NUMERIC(18,2)  NOT NULL DEFAULT 0,
    exemption_total              NUMERIC(18,2)  NOT NULL DEFAULT 0,
    other_deductions_total       NUMERIC(18,2)  NOT NULL DEFAULT 0,
    employer_cost_total          NUMERIC(18,2)  NOT NULL DEFAULT 0,
    -- Satır denetimleri (brüt − kesintiler ≠ net gibi), [{row, code, message}].
    issues                       JSONB          NOT NULL DEFAULT '[]'::jsonb,
    journal_draft_id             BIGINT         NULL REFERENCES journal_drafts (id) ON DELETE SET NULL,
    created_by                   BIGINT         NULL REFERENCES users (id) ON DELETE SET NULL,
    created_at                   TIMESTAMPTZ    NOT NULL DEFAULT NOW(),
    updated_at                   TIMESTAMPTZ    NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_payroll_runs_uuid UNIQUE (uuid),
    CONSTRAINT uq_payroll_runs_taxpayer_id UNIQUE (taxpayer_id, id),
    CONSTRAINT uq_payroll_runs_period UNIQUE (taxpayer_id, period),
    CONSTRAINT chk_payroll_runs_period CHECK (period ~ '^[0-9]{4}-(0[1-9]|1[0-2])$'),
    CONSTRAINT chk_payroll_runs_source CHECK (source IN ('import', 'manual', 'demo')),
    CONSTRAINT chk_payroll_runs_source_system CHECK (source_system IN ('luca', 'zirve', 'logo', 'generic'))
);

CREATE TRIGGER trg_payroll_runs_set_updated_at
    BEFORE UPDATE ON payroll_runs FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- Tutarlar bordrodaki gibidir: income_tax / stamp_tax istisna düşülmüş,
-- ödenecek vergidir; istisnalar ayrıca saklanır. agi yalnızca 2022 öncesi
-- bordrolarda dolu olur (net'e eklenir).
CREATE TABLE payroll_lines (
    id                     BIGSERIAL      PRIMARY KEY,
    taxpayer_id            BIGINT         NOT NULL,
    payroll_run_id         BIGINT         NOT NULL,
    employee_id            BIGINT         NULL,
    line_no                INT            NOT NULL,
    employee_name          TEXT           NOT NULL,
    national_id_masked     VARCHAR(11)    NULL,
    days_worked            INT            NULL,
    gross                  NUMERIC(18,2)  NOT NULL DEFAULT 0,
    sgk_base               NUMERIC(18,2)  NOT NULL DEFAULT 0,
    sgk_employee           NUMERIC(18,2)  NOT NULL DEFAULT 0,
    unemployment_employee  NUMERIC(18,2)  NOT NULL DEFAULT 0,
    income_tax_base        NUMERIC(18,2)  NOT NULL DEFAULT 0,
    income_tax             NUMERIC(18,2)  NOT NULL DEFAULT 0,
    income_tax_exemption   NUMERIC(18,2)  NOT NULL DEFAULT 0,
    stamp_tax              NUMERIC(18,2)  NOT NULL DEFAULT 0,
    stamp_tax_exemption    NUMERIC(18,2)  NOT NULL DEFAULT 0,
    agi                    NUMERIC(18,2)  NOT NULL DEFAULT 0,
    other_deductions       NUMERIC(18,2)  NOT NULL DEFAULT 0,
    net                    NUMERIC(18,2)  NOT NULL DEFAULT 0,
    sgk_employer           NUMERIC(18,2)  NOT NULL DEFAULT 0,
    unemployment_employer  NUMERIC(18,2)  NOT NULL DEFAULT 0,
    sgk_incentive          NUMERIC(18,2)  NOT NULL DEFAULT 0,
    employer_cost          NUMERIC(18,2)  NOT NULL DEFAULT 0,
    created_at             TIMESTAMPTZ    NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_payroll_lines_line UNIQUE (payroll_run_id, line_no),
    CONSTRAINT fk_payroll_lines_run
        FOREIGN KEY (taxpayer_id, payroll_run_id) REFERENCES payroll_runs (taxpayer_id, id) ON DELETE CASCADE,
    CONSTRAINT fk_payroll_lines_employee
        FOREIGN KEY (taxpayer_id, employee_id) REFERENCES employees (taxpayer_id, id) ON DELETE RESTRICT,
    CONSTRAINT chk_payroll_lines_masked CHECK (national_id_masked IS NULL OR national_id_masked ~ '^[0-9]{3}\*{6}[0-9]{2}$'),
    CONSTRAINT chk_payroll_lines_days CHECK (days_worked IS NULL OR (days_worked >= 0 AND days_worked <= 31)),
    CONSTRAINT chk_payroll_lines_amounts CHECK (
        gross >= 0 AND sgk_base >= 0 AND sgk_employee >= 0 AND unemployment_employee >= 0 AND income_tax_base >= 0
        AND income_tax >= 0 AND income_tax_exemption >= 0 AND stamp_tax >= 0 AND stamp_tax_exemption >= 0
        AND agi >= 0 AND other_deductions >= 0 AND net >= 0 AND sgk_employer >= 0 AND unemployment_employer >= 0
        AND sgk_incentive >= 0 AND employer_cost >= 0)
);

CREATE INDEX idx_payroll_lines_employee ON payroll_lines (taxpayer_id, employee_id);

-- Dönemin SGK tahakkuku ve ödemesi. payable = accrual − incentive.
CREATE TABLE sgk_premiums (
    id                   BIGSERIAL      PRIMARY KEY,
    uuid                 UUID           NOT NULL DEFAULT gen_random_uuid(),
    taxpayer_id          BIGINT         NOT NULL REFERENCES taxpayers (id) ON DELETE RESTRICT,
    period               CHAR(7)        NOT NULL,
    payroll_run_id       BIGINT         NULL,
    accrual_amount       NUMERIC(18,2)  NOT NULL DEFAULT 0,
    incentive_amount     NUMERIC(18,2)  NOT NULL DEFAULT 0,
    payable_amount       NUMERIC(18,2)  NOT NULL DEFAULT 0,
    due_date             DATE           NOT NULL,
    document_no          TEXT           NULL,
    paid_amount          NUMERIC(18,2)  NOT NULL DEFAULT 0,
    paid_at              DATE           NULL,
    bank_transaction_id  BIGINT         NULL,
    note                 TEXT           NULL,
    created_at           TIMESTAMPTZ    NOT NULL DEFAULT NOW(),
    updated_at           TIMESTAMPTZ    NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_sgk_premiums_uuid UNIQUE (uuid),
    CONSTRAINT uq_sgk_premiums_period UNIQUE (taxpayer_id, period),
    CONSTRAINT fk_sgk_premiums_run
        FOREIGN KEY (taxpayer_id, payroll_run_id) REFERENCES payroll_runs (taxpayer_id, id) ON DELETE SET NULL (payroll_run_id),
    CONSTRAINT fk_sgk_premiums_bank_transaction
        FOREIGN KEY (bank_transaction_id) REFERENCES bank_transactions (id) ON DELETE SET NULL,
    CONSTRAINT chk_sgk_premiums_period CHECK (period ~ '^[0-9]{4}-(0[1-9]|1[0-2])$'),
    CONSTRAINT chk_sgk_premiums_amounts CHECK (accrual_amount >= 0 AND incentive_amount >= 0 AND payable_amount >= 0 AND paid_amount >= 0),
    CONSTRAINT chk_sgk_premiums_paid CHECK ((paid_at IS NULL) = (paid_amount = 0))
);

CREATE TRIGGER trg_sgk_premiums_set_updated_at
    BEFORE UPDATE ON sgk_premiums FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- Bordro dökümü ve bordro pusulası PDF'leri (belge olarak saklanır).
CREATE TABLE payroll_documents (
    id                   BIGSERIAL    PRIMARY KEY,
    uuid                 UUID         NOT NULL DEFAULT gen_random_uuid(),
    taxpayer_id          BIGINT       NOT NULL REFERENCES taxpayers (id) ON DELETE RESTRICT,
    period               CHAR(7)      NOT NULL,
    payroll_run_id       BIGINT       NULL,
    employee_id          BIGINT       NULL,
    kind                 TEXT         NOT NULL DEFAULT 'payroll',
    title                TEXT         NOT NULL DEFAULT '',
    storage_key          TEXT         NOT NULL,
    sha256               CHAR(64)     NOT NULL,
    size_bytes           BIGINT       NOT NULL,
    content_type         TEXT         NOT NULL DEFAULT 'application/pdf',
    file_name            TEXT         NOT NULL,
    visible_to_taxpayer  BOOLEAN      NOT NULL DEFAULT TRUE,
    created_by           BIGINT       NULL REFERENCES users (id) ON DELETE SET NULL,
    created_at           TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_payroll_documents_uuid UNIQUE (uuid),
    CONSTRAINT uq_payroll_documents_sha256 UNIQUE (taxpayer_id, sha256),
    CONSTRAINT fk_payroll_documents_run
        FOREIGN KEY (taxpayer_id, payroll_run_id) REFERENCES payroll_runs (taxpayer_id, id) ON DELETE SET NULL (payroll_run_id),
    CONSTRAINT fk_payroll_documents_employee
        FOREIGN KEY (taxpayer_id, employee_id) REFERENCES employees (taxpayer_id, id) ON DELETE SET NULL (employee_id),
    CONSTRAINT chk_payroll_documents_period CHECK (period ~ '^[0-9]{4}-(0[1-9]|1[0-2])$'),
    CONSTRAINT chk_payroll_documents_kind CHECK (kind IN ('payroll', 'payslip', 'sgk_accrual', 'sgk_receipt', 'other')),
    CONSTRAINT chk_payroll_documents_sha256 CHECK (sha256 ~ '^[0-9a-f]{64}$'),
    CONSTRAINT chk_payroll_documents_size CHECK (size_bytes > 0)
);

CREATE INDEX idx_payroll_documents_list ON payroll_documents (taxpayer_id, period DESC, created_at DESC);

SELECT rls_apply_taxpayer('employees');
SELECT rls_apply_taxpayer('payroll_runs');
SELECT rls_apply_taxpayer('payroll_lines');
SELECT rls_apply_taxpayer('sgk_premiums');
SELECT rls_apply_taxpayer('payroll_documents');

-- Fiş taslağı satırının kaynağı bordro olabilir; banka hareketi bordroya
-- (net maaş ödemesi) ya da SGK tahakkukuna eşlenebilir.
ALTER TABLE journal_draft_lines DROP CONSTRAINT chk_journal_draft_lines_source_kind;
ALTER TABLE journal_draft_lines ADD CONSTRAINT chk_journal_draft_lines_source_kind
    CHECK (source_kind IS NULL OR source_kind IN ('invoice', 'invoice_item', 'bank_transaction', 'receipt', 'manual', 'payroll'));

ALTER TABLE bank_transaction_matches DROP CONSTRAINT chk_bank_transaction_matches_target_type;
ALTER TABLE bank_transaction_matches ADD CONSTRAINT chk_bank_transaction_matches_target_type
    CHECK (target_type IN ('invoice', 'payroll_run', 'sgk_premium'));

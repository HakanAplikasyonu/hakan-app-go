-- Faturalama (issue #52, D9 + D14): ofis taban ücreti + aktif mükellef başına.
-- İlk sürümde tahsilat yok; plan, aylık kullanım ve fatura taslağı tutulur.
-- Tutarlar plan kaydından gelir; fiyatlar ürün kararıdır (PR "Karar gerekiyor").

CREATE TABLE billing_plans (
    id                 BIGSERIAL      PRIMARY KEY,
    code               TEXT           NOT NULL,
    name               TEXT           NOT NULL,
    currency           CHAR(3)        NOT NULL DEFAULT 'TRY',
    base_fee           NUMERIC(18,2)  NOT NULL DEFAULT 0,
    per_taxpayer_fee   NUMERIC(18,2)  NOT NULL DEFAULT 0,
    -- Taban ücrete dahil mükellef sayısı; üstü mükellef başına ücretlenir.
    included_taxpayers INT            NOT NULL DEFAULT 0,
    vat_rate           NUMERIC(5,2)   NOT NULL DEFAULT 20,
    active             BOOLEAN        NOT NULL DEFAULT TRUE,
    created_at         TIMESTAMPTZ    NOT NULL DEFAULT NOW(),
    updated_at         TIMESTAMPTZ    NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_billing_plans_code UNIQUE (code),
    CONSTRAINT chk_billing_plans_code CHECK (code ~ '^[a-z0-9][a-z0-9_-]{0,31}$'),
    CONSTRAINT chk_billing_plans_fees CHECK (base_fee >= 0 AND per_taxpayer_fee >= 0 AND included_taxpayers >= 0),
    CONSTRAINT chk_billing_plans_vat CHECK (vat_rate >= 0 AND vat_rate <= 100)
);

CREATE TRIGGER trg_billing_plans_set_updated_at
    BEFORE UPDATE ON billing_plans FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- Fiyatlar karar verilene kadar sıfır: taslaklar sayımı gösterir, tutar 0.
INSERT INTO billing_plans (code, name) VALUES ('standart', 'Standart');

-- Ofisin planı ve salt okunur kipi (AccessPolicy). Satır yoksa varsayılan plan.
CREATE TABLE organization_billing (
    organization_id  BIGINT       PRIMARY KEY REFERENCES organizations (id) ON DELETE CASCADE,
    plan_id          BIGINT       NOT NULL REFERENCES billing_plans (id) ON DELETE RESTRICT,
    read_only        BOOLEAN      NOT NULL DEFAULT FALSE,
    read_only_reason TEXT         NOT NULL DEFAULT '',
    updated_by       BIGINT       NULL REFERENCES users (id) ON DELETE SET NULL,
    created_at       TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at       TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);

CREATE TRIGGER trg_organization_billing_set_updated_at
    BEFORE UPDATE ON organization_billing FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- Aylık fatura taslağı: o ayın aktif mükellef sayımı ve hesap. Plan anlık
-- görüntüsü saklanır, plan sonradan değişse de taslak değişmez.
CREATE TABLE billing_statements (
    id                  BIGSERIAL      PRIMARY KEY,
    uuid                UUID           NOT NULL DEFAULT gen_random_uuid(),
    organization_id     BIGINT         NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    period              CHAR(7)        NOT NULL,
    plan_code           TEXT           NOT NULL,
    plan_snapshot       JSONB          NOT NULL,
    active_taxpayers    INT            NOT NULL,
    billable_taxpayers  INT            NOT NULL,
    currency            CHAR(3)        NOT NULL,
    base_fee            NUMERIC(18,2)  NOT NULL,
    taxpayer_fee_total  NUMERIC(18,2)  NOT NULL,
    subtotal            NUMERIC(18,2)  NOT NULL,
    vat_rate            NUMERIC(5,2)   NOT NULL,
    vat_amount          NUMERIC(18,2)  NOT NULL,
    total               NUMERIC(18,2)  NOT NULL,
    status              TEXT           NOT NULL DEFAULT 'draft',
    lines               JSONB          NOT NULL DEFAULT '[]'::jsonb,
    taxpayers           JSONB          NOT NULL DEFAULT '[]'::jsonb,
    generated_at        TIMESTAMPTZ    NOT NULL DEFAULT NOW(),
    created_at          TIMESTAMPTZ    NOT NULL DEFAULT NOW(),
    updated_at          TIMESTAMPTZ    NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_billing_statements_uuid UNIQUE (uuid),
    CONSTRAINT uq_billing_statements_period UNIQUE (organization_id, period),
    CONSTRAINT chk_billing_statements_period CHECK (period ~ '^[0-9]{4}-(0[1-9]|1[0-2])$'),
    CONSTRAINT chk_billing_statements_status CHECK (status IN ('draft', 'final', 'void'))
);

CREATE TRIGGER trg_billing_statements_set_updated_at
    BEFORE UPDATE ON billing_statements FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- RLS (issue #6): ofis satırları ofis bağlamıyla; planlar platformun.
SELECT rls_apply_org('organization_billing'::regclass);
SELECT rls_apply_org('billing_statements'::regclass);

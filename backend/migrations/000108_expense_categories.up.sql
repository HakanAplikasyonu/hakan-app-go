-- Masraf kategorileri ve fiş muhasebeleştirme (issue #384).
--
-- expense_categories: ofis şablonu (engagement_id NULL) ve mükellefe özel
-- (engagement_id dolu) kategori. Satır yoksa platform varsayılanları
-- (receipts/posting.DefaultCategories) kullanılır; aynı code ile yazılan
-- satır üsttekini ezer (mükellef > ofis > platform). is_active = false bir
-- varsayılanı o kapsamda gizler.
CREATE TABLE expense_categories (
    id               BIGSERIAL    PRIMARY KEY,
    uuid             UUID         NOT NULL DEFAULT gen_random_uuid(),
    organization_id  BIGINT       NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    engagement_id    BIGINT       NULL,
    code             VARCHAR(48)  NOT NULL,
    name             TEXT         NOT NULL,
    account_code     VARCHAR(64)  NOT NULL,
    vat_account_code VARCHAR(64)  NOT NULL DEFAULT '191',
    -- no: KDV indirilmez, gidere eklenir (KKEG vb.).
    deductible       TEXT         NOT NULL DEFAULT 'yes',
    -- Otomatik seçim ipuçları: fiş türü ve satıcı adında geçen kelimeler.
    receipt_types    TEXT[]       NOT NULL DEFAULT '{}',
    keywords         TEXT[]       NOT NULL DEFAULT '{}',
    is_active        BOOLEAN      NOT NULL DEFAULT TRUE,
    created_by       BIGINT       NULL REFERENCES users (id) ON DELETE SET NULL,
    created_at       TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at       TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_expense_categories_uuid UNIQUE (uuid),
    CONSTRAINT uq_expense_categories_code UNIQUE NULLS NOT DISTINCT (organization_id, engagement_id, code),
    CONSTRAINT fk_expense_categories_engagement
        FOREIGN KEY (organization_id, engagement_id) REFERENCES engagements (organization_id, id) ON DELETE CASCADE,
    CONSTRAINT chk_expense_categories_code CHECK (code ~ '^[a-z0-9_]{1,48}$'),
    CONSTRAINT chk_expense_categories_name CHECK (btrim(name) <> ''),
    CONSTRAINT chk_expense_categories_account CHECK (account_code ~ '^[0-9]{3}(\.[0-9A-Za-z]+)*$'),
    CONSTRAINT chk_expense_categories_vat_account CHECK (vat_account_code ~ '^[0-9]{3}(\.[0-9A-Za-z]+)*$'),
    CONSTRAINT chk_expense_categories_deductible CHECK (deductible IN ('yes', 'no'))
);

CREATE TRIGGER trg_expense_categories_set_updated_at
    BEFORE UPDATE ON expense_categories FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- receipt_postings: masraf / Z raporu kaydının ofis tarafı (iş ürünü, D2):
-- seçilen kategori, önerilen ya da müşavirin yazdığı hesap kodu, önerinin
-- kaynağı ve onayda üretilen yevmiye taslağı. Kaynak satır (expenses /
-- retail_sales) mükellefe aittir; bu tablo engagement kapsamlıdır.
CREATE TABLE receipt_postings (
    id               BIGSERIAL     PRIMARY KEY,
    organization_id  BIGINT        NOT NULL,
    engagement_id    BIGINT        NOT NULL,
    taxpayer_id      BIGINT        NOT NULL REFERENCES taxpayers (id) ON DELETE RESTRICT,
    target_type      TEXT          NOT NULL,
    target_id        BIGINT        NOT NULL,
    category_code    VARCHAR(48)   NULL,
    account_code     VARCHAR(64)   NULL,
    -- category | vendor_memory | plan | manual
    account_source   TEXT          NULL,
    confidence       NUMERIC(5,4)  NULL,
    journal_draft_id BIGINT        NULL,
    decided_by       BIGINT        NULL REFERENCES users (id) ON DELETE SET NULL,
    created_at       TIMESTAMPTZ   NOT NULL DEFAULT NOW(),
    updated_at       TIMESTAMPTZ   NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_receipt_postings_target UNIQUE (engagement_id, target_type, target_id),
    CONSTRAINT fk_receipt_postings_engagement
        FOREIGN KEY (organization_id, engagement_id) REFERENCES engagements (organization_id, id) ON DELETE CASCADE,
    CONSTRAINT fk_receipt_postings_journal
        FOREIGN KEY (engagement_id, journal_draft_id) REFERENCES journal_drafts (engagement_id, id) ON DELETE SET NULL (journal_draft_id),
    CONSTRAINT chk_receipt_postings_target CHECK (target_type IN ('expense', 'retail_sale')),
    CONSTRAINT chk_receipt_postings_account CHECK (account_code IS NULL OR account_code ~ '^[0-9]{3}(\.[0-9A-Za-z]+)*$'),
    CONSTRAINT chk_receipt_postings_source CHECK (account_source IS NULL OR account_source IN ('category', 'vendor_memory', 'plan', 'manual')),
    CONSTRAINT chk_receipt_postings_confidence CHECK (confidence IS NULL OR (confidence >= 0 AND confidence <= 1))
);

CREATE INDEX idx_receipt_postings_category ON receipt_postings (organization_id, engagement_id, category_code);

CREATE TRIGGER trg_receipt_postings_set_updated_at
    BEFORE UPDATE ON receipt_postings FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- Masraf listesi: dönem + onay durumu + tarih sırası.
CREATE INDEX idx_expenses_list ON expenses (taxpayer_id, approval_status, expense_date DESC);

-- RLS: kategoriler ofis kapsamlı (şablon satırında engagement yok),
-- muhasebeleştirme engagement kapsamlı. Şablon FORCE da açar (000086).
SELECT rls_apply_org('expense_categories'::regclass);
SELECT rls_apply_engagement('receipt_postings'::regclass);

-- Defter türü ve işletme hesabı (#780). Ofisin mükelleflerinin bir kısmı
-- işletme defteri (gelir-gider usulü) ya da serbest meslek kazanç defteri
-- tutar; hesap planı (600/391/120 ...) yalnız bilanço usulünde kullanılır.
--
-- ledger_type ofis <-> mükellef hizmet ilişkisinde (engagement) durur: ofis
-- düzenler, satır RLS kapsamındadır. Mevcut satırlar bilançodur.
ALTER TABLE engagements
    ADD COLUMN ledger_type TEXT NOT NULL DEFAULT 'bilanco',
    ADD CONSTRAINT chk_engagements_ledger_type CHECK (ledger_type IN ('bilanco', 'isletme', 'serbest_meslek'));

-- İşletme hesabı özeti için yıl başı / yıl sonu mal mevcudu (işletme
-- defteri). Kâr = gelir - gider - dönem başı mal + dönem sonu mal.
CREATE TABLE operating_account_inventories (
    id                BIGSERIAL     PRIMARY KEY,
    organization_id   BIGINT        NOT NULL,
    engagement_id     BIGINT        NOT NULL,
    year              INT           NOT NULL,
    opening_inventory NUMERIC(18,2) NOT NULL DEFAULT 0,
    closing_inventory NUMERIC(18,2) NULL,
    updated_by        BIGINT        NULL REFERENCES users (id) ON DELETE SET NULL,
    created_at        TIMESTAMPTZ   NOT NULL DEFAULT NOW(),
    updated_at        TIMESTAMPTZ   NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_operating_account_inventories UNIQUE (organization_id, engagement_id, year),
    CONSTRAINT chk_operating_account_inventories_year CHECK (year BETWEEN 2000 AND 2100),
    CONSTRAINT chk_operating_account_inventories_amounts CHECK (
        opening_inventory >= 0 AND (closing_inventory IS NULL OR closing_inventory >= 0)),
    CONSTRAINT fk_operating_account_inventories_engagement
        FOREIGN KEY (organization_id, engagement_id) REFERENCES engagements (organization_id, id) ON DELETE CASCADE
);

CREATE TRIGGER trg_operating_account_inventories_set_updated_at
    BEFORE UPDATE ON operating_account_inventories
    FOR EACH ROW
    EXECUTE FUNCTION set_updated_at();

CREATE INDEX idx_operating_account_inventories_updated_by ON operating_account_inventories (updated_by);

SELECT rls_apply_engagement('operating_account_inventories'::regclass);

-- Bir belgenin gelir / gider kategorisini ofis değiştirebilir (ör. alış
-- faturası "Mal alışı" yerine "Genel gider"). Belge mükellefe aittir;
-- kategori ofisin işidir, engagement kapsamlı.
CREATE TABLE operating_account_categories (
    id              BIGSERIAL   PRIMARY KEY,
    organization_id BIGINT      NOT NULL,
    engagement_id   BIGINT      NOT NULL,
    source          TEXT        NOT NULL,
    record_uuid     UUID        NOT NULL,
    category        TEXT        NOT NULL,
    decided_by      BIGINT      NULL REFERENCES users (id) ON DELETE SET NULL,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_operating_account_categories UNIQUE (organization_id, engagement_id, source, record_uuid),
    CONSTRAINT chk_operating_account_categories_source CHECK (source IN ('invoice', 'expense', 'retail_sale')),
    CONSTRAINT chk_operating_account_categories_category CHECK (category ~ '^[a-z_]{2,40}$'),
    CONSTRAINT fk_operating_account_categories_engagement
        FOREIGN KEY (organization_id, engagement_id) REFERENCES engagements (organization_id, id) ON DELETE CASCADE
);

CREATE TRIGGER trg_operating_account_categories_set_updated_at
    BEFORE UPDATE ON operating_account_categories
    FOR EACH ROW
    EXECUTE FUNCTION set_updated_at();

CREATE INDEX idx_operating_account_categories_decided_by ON operating_account_categories (decided_by);

SELECT rls_apply_engagement('operating_account_categories'::regclass);

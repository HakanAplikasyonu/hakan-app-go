-- Luca hesap planı ve cari alt hesap CSV çıktısı (#464).
--
-- 1. generated_subaccounts: portalın bir cari (müşteri / satıcı) için ana
--    hesap (120, 320, 159, 329 …) altında açtığı alt hesap kodu. Kod mükellef
--    (engagement) ve cari ile kalıcı eşlenir: aynı cari aynı ana hesapta
--    ikinci kez yeni kod almaz, bir kod iki cariye verilmez. Kod, mükellefin
--    son içe aktarılan Luca hesap planıyla çakışmayan sıradaki koddur.
-- 2. account_plan_exports: hesap planı dosyası indirme geçmişi ("yeni
--    hesaplar" ya da "tüm hesap planı"), karşılaştırılan plan sürümü, şablon
--    sürümü ve dosyanın SHA-256'sı. Dosya her istekte yeniden üretilir.
--
-- Ofis iş ürünü: organization_id + engagement_id, engagement kapsamlı RLS.

CREATE TABLE generated_subaccounts (
    id                BIGSERIAL    PRIMARY KEY,
    uuid              UUID         NOT NULL DEFAULT gen_random_uuid(),
    organization_id   BIGINT       NOT NULL,
    engagement_id     BIGINT       NOT NULL,
    taxpayer_id       BIGINT       NOT NULL REFERENCES taxpayers (id) ON DELETE RESTRICT,
    target            TEXT         NOT NULL DEFAULT 'luca',
    main_code         CHAR(3)      NOT NULL,
    code              VARCHAR(64)  NOT NULL,
    -- Kodun asıldığı üst hesap (120.01); planda yoksa yeni hesaplar dosyasına
    -- o da girer.
    parent_code       VARCHAR(64)  NOT NULL,
    -- Carinin anahtarı: VKN / TCKN, yoksa "customer:<uuid>".
    counterparty_key  TEXT         NOT NULL,
    -- Cari mükellef verisidir (taxpayer kapsamı): yalnızca uuid'si tutulur.
    customer_uuid     UUID         NULL,
    name              TEXT         NOT NULL,
    created_by        BIGINT       NULL REFERENCES users (id) ON DELETE SET NULL,
    created_at        TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_generated_subaccounts_uuid UNIQUE (uuid),
    CONSTRAINT uq_generated_subaccounts_counterparty UNIQUE (engagement_id, target, main_code, counterparty_key),
    CONSTRAINT uq_generated_subaccounts_code UNIQUE (engagement_id, target, code),
    CONSTRAINT fk_generated_subaccounts_engagement
        FOREIGN KEY (organization_id, engagement_id) REFERENCES engagements (organization_id, id) ON DELETE CASCADE,
    CONSTRAINT chk_generated_subaccounts_target CHECK (target ~ '^[a-z][a-z0-9_]*$'),
    CONSTRAINT chk_generated_subaccounts_main CHECK (main_code ~ '^[0-9]{3}$'),
    CONSTRAINT chk_generated_subaccounts_code CHECK (code ~ '^[0-9]{3}(\.[0-9]+)+$' AND left(code, 3) = main_code),
    CONSTRAINT chk_generated_subaccounts_parent CHECK (code LIKE parent_code || '.%'),
    CONSTRAINT chk_generated_subaccounts_key CHECK (btrim(counterparty_key) <> ''),
    CONSTRAINT chk_generated_subaccounts_name CHECK (btrim(name) <> '')
);

CREATE INDEX idx_generated_subaccounts_list ON generated_subaccounts (organization_id, engagement_id, target, code);

SELECT rls_apply_engagement('generated_subaccounts'::regclass);

CREATE TABLE account_plan_exports (
    id                  BIGSERIAL    PRIMARY KEY,
    uuid                UUID         NOT NULL DEFAULT gen_random_uuid(),
    organization_id     BIGINT       NOT NULL,
    engagement_id       BIGINT       NOT NULL,
    target              TEXT         NOT NULL DEFAULT 'luca',
    -- new: yalnızca Luca'da henüz olmayan hesaplar; full: tüm hesap planı.
    kind                TEXT         NOT NULL,
    template_version    TEXT         NOT NULL,
    template_verified   BOOLEAN      NOT NULL,
    -- Karşılaştırılan içe aktarma (son Luca hesap planı / mizan dökümü).
    compared_import_id  BIGINT       NULL REFERENCES account_plan_imports (id) ON DELETE SET NULL,
    file_name           TEXT         NOT NULL,
    sha256              CHAR(64)     NOT NULL,
    size                INT          NOT NULL,
    account_count       INT          NOT NULL,
    created_by          BIGINT       NULL REFERENCES users (id) ON DELETE SET NULL,
    created_at          TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_account_plan_exports_uuid UNIQUE (uuid),
    CONSTRAINT fk_account_plan_exports_engagement
        FOREIGN KEY (organization_id, engagement_id) REFERENCES engagements (organization_id, id) ON DELETE CASCADE,
    CONSTRAINT chk_account_plan_exports_kind CHECK (kind IN ('new', 'full')),
    CONSTRAINT chk_account_plan_exports_target CHECK (target ~ '^[a-z][a-z0-9_]*$'),
    CONSTRAINT chk_account_plan_exports_counts CHECK (size >= 0 AND account_count >= 0)
);

CREATE INDEX idx_account_plan_exports_list ON account_plan_exports (organization_id, engagement_id, created_at DESC);
CREATE INDEX idx_account_plan_exports_import ON account_plan_exports (compared_import_id);

SELECT rls_apply_engagement('account_plan_exports'::regclass);

-- Beyanname formatına uygunluk (#390): KDV1, MUHSGK ve Ba/Bs dönem beyanları.
--
-- Beyan önizlemesi ortak veriden (faturalar, onaylı masraflar ve Z raporları,
-- bordro ve SGK tahakkuku) her istekte hesaplanır; bu tablo yalnızca ofisin
-- dönem kararını tutar: düzeltme alanları (devreden KDV, ilave edilecek KDV,
-- diğer indirimler), onaylanan kontrol maddeleri, "hazırlandı" anındaki
-- donmuş görüntü (snapshot) ve "verildi" bilgisi (tarih, tahakkuk fiş no).
--
-- Ofis iş ürünü: organization_id + engagement_id, engagement kapsamlı RLS
-- (vat_carry_forwards gibi). Mükellef başka ofise geçerse beyan kayıtları
-- eski ofiste kalır.

-- roles ve role_permissions FORCE altında (000086): izin kopyaları ofis
-- rollerine ancak sistem kipinde yazılır (DATABASE_RULES).
SET LOCAL app.system = 'on';

INSERT INTO permissions (name, slug) VALUES
    ('Read tenant tax returns', 'tenant.tax_returns.read'),
    ('Write tenant tax returns', 'tenant.tax_returns.write')
ON CONFLICT (slug) DO NOTHING;

-- MUHSGK bordrodan beslenir (maaş verisi): bordro izinleri gibi stajyer
-- görmez (sahip kendiliğinden alır, 000044).
SELECT grant_tenant_role_permission(r, p)
FROM unnest(ARRAY['admin', 'accountant', 'member']) AS r,
     unnest(ARRAY['tenant.tax_returns.read', 'tenant.tax_returns.write']) AS p;

CREATE TABLE tax_returns (
    id              BIGSERIAL    PRIMARY KEY,
    uuid            UUID         NOT NULL DEFAULT gen_random_uuid(),
    organization_id BIGINT       NOT NULL,
    engagement_id   BIGINT       NOT NULL,
    -- kdv1: 1 No.lu KDV; muhsgk: muhtasar ve prim hizmet; babs: Form Ba + Bs.
    kind            TEXT         NOT NULL,
    period          CHAR(7)      NOT NULL,
    -- draft → prepared (snapshot donar) → filed (verildi). prepared → draft
    -- geri açılabilir; filed son durumdur (düzeltme beyannamesi kapsam dışı).
    status          TEXT         NOT NULL DEFAULT 'draft',
    -- Elle girilen düzeltmeler ({"carried_in": "1250.00", ...}; tutarlar
    -- metin, iki ondalık).
    adjustments     JSONB        NOT NULL DEFAULT '{}'::jsonb,
    -- Onaylanan uyarı kodları (["PENDING_EXPENSES", ...]).
    acknowledged    JSONB        NOT NULL DEFAULT '[]'::jsonb,
    -- Hazırlandığı andaki önizleme (API yanıtının aynısı).
    snapshot        JSONB        NULL,
    -- Beyanın ödenecek / devreden özeti (liste ekranı için snapshot'tan).
    payable_amount  NUMERIC(18,2) NULL,
    carry_out       NUMERIC(18,2) NULL,
    filed_on        DATE         NULL,
    accrual_number  TEXT         NULL,
    note            TEXT         NULL,
    prepared_by     BIGINT       NULL REFERENCES users (id) ON DELETE SET NULL,
    prepared_at     TIMESTAMPTZ  NULL,
    filed_by        BIGINT       NULL REFERENCES users (id) ON DELETE SET NULL,
    filed_at        TIMESTAMPTZ  NULL,
    created_by      BIGINT       NULL REFERENCES users (id) ON DELETE SET NULL,
    created_at      TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_tax_returns_uuid UNIQUE (uuid),
    CONSTRAINT uq_tax_returns_period UNIQUE (organization_id, engagement_id, kind, period),
    CONSTRAINT fk_tax_returns_engagement
        FOREIGN KEY (organization_id, engagement_id) REFERENCES engagements (organization_id, id) ON DELETE CASCADE,
    CONSTRAINT chk_tax_returns_kind CHECK (kind IN ('kdv1', 'muhsgk', 'babs')),
    CONSTRAINT chk_tax_returns_status CHECK (status IN ('draft', 'prepared', 'filed')),
    CONSTRAINT chk_tax_returns_period CHECK (period ~ '^[0-9]{4}-(0[1-9]|1[0-2])$'),
    CONSTRAINT chk_tax_returns_adjustments CHECK (jsonb_typeof(adjustments) = 'object'),
    CONSTRAINT chk_tax_returns_acknowledged CHECK (jsonb_typeof(acknowledged) = 'array'),
    CONSTRAINT chk_tax_returns_snapshot CHECK (status = 'draft' OR snapshot IS NOT NULL),
    CONSTRAINT chk_tax_returns_filed CHECK (status <> 'filed' OR filed_on IS NOT NULL)
);

CREATE INDEX idx_tax_returns_list ON tax_returns (organization_id, engagement_id, period DESC, kind);

CREATE TRIGGER trg_tax_returns_set_updated_at
    BEFORE UPDATE ON tax_returns FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- RLS (issue #6): engagement kapsamlı.
SELECT rls_apply_engagement('tax_returns'::regclass);

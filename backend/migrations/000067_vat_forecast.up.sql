-- KDV erken uyarı ve beyan takvimi (issue #33, docs/00 K9 / T10).
--
-- Tahmin bir tablo değildir: dönem içi hesaplanan / indirilecek KDV her
-- istekte invoices + invoice_tax_summaries (mükellefin kaynak verisi) üzerinden
-- hesaplanır. Burada yalnızca ofisin iş ürünü olan iki şey tutulur:
--
--   * vat_carry_forwards: elle girilen açılış devreden KDV'si. Henüz defter
--     (yevmiye) yok; sistemdeki ilk dönemden önceki devir, ya da beyanname ile
--     sistem verisi ayrıştığında müşavirin düzelttiği tutar buraya yazılır.
--     Satır "bu döneme önceki dönemden devreden KDV şudur" demektir; sonraki
--     dönemlerin devri bu noktadan fatura verisiyle yürütülür.
--   * İki uyarı kodu. Eşikler ve beyan günü warning_rule_configs üzerinden
--     ofis / mükellef bazında ayarlanır (docs/00 K9).

CREATE TABLE vat_carry_forwards (
    id              BIGSERIAL      PRIMARY KEY,
    organization_id BIGINT         NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    engagement_id   BIGINT         NOT NULL,
    period          CHAR(7)        NOT NULL,
    amount          NUMERIC(18,2)  NOT NULL,
    note            TEXT           NULL,
    updated_by      BIGINT         NULL REFERENCES users (id) ON DELETE SET NULL,
    created_at      TIMESTAMPTZ    NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ    NOT NULL DEFAULT NOW(),
    CONSTRAINT fk_vat_carry_forwards_engagement
        FOREIGN KEY (organization_id, engagement_id) REFERENCES engagements (organization_id, id) ON DELETE CASCADE,
    CONSTRAINT uq_vat_carry_forwards_period UNIQUE (organization_id, engagement_id, period),
    CONSTRAINT chk_vat_carry_forwards_period CHECK (period ~ '^[0-9]{4}-(0[1-9]|1[0-2])$'),
    CONSTRAINT chk_vat_carry_forwards_amount CHECK (amount >= 0)
);

CREATE TRIGGER trg_vat_carry_forwards_set_updated_at
    BEFORE UPDATE ON vat_carry_forwards FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- Uyarı kodları (catalog.go ile aynı; catalog_test.go denetler).
--   VAT_PAYABLE_THRESHOLD: dönemin ödenecek KDV tahmini limit'e ulaştı.
--     limit NULL: ofis girmeden kural çalışmaz.
--   VAT_DEADLINE_NEAR: KDV beyannamesinin son gününe days_before gün ya da daha
--     az kaldı. due_day beyan takvimidir (izleyen ayın günü, varsayılan 28).
INSERT INTO warning_codes (code, section, default_severity, default_params) VALUES
    ('VAT_PAYABLE_THRESHOLD',   '00', 'warning', '{"limit": null}'),
    ('VAT_DEADLINE_NEAR',       '00', 'warning', '{"days_before": 5, "due_day": 28}');

-- RLS (issue #6).
SELECT rls_apply_engagement('vat_carry_forwards'::regclass);

INSERT INTO permissions (name, slug) VALUES
    ('Read tenant VAT forecast', 'tenant.vat.read'),
    ('Write tenant VAT forecast', 'tenant.vat.write')
ON CONFLICT (slug) DO NOTHING;

-- Sahip yeni tenant.* iznini kendiliğinden alır (000044). Okuma: yönetici,
-- mali müşavir, personel, stajyer. Yazma (uyarıyı çalıştırma, açılış devri):
-- yönetici, mali müşavir, personel.
SELECT grant_tenant_role_permission('admin', 'tenant.vat.read');
SELECT grant_tenant_role_permission('accountant', 'tenant.vat.read');
SELECT grant_tenant_role_permission('member', 'tenant.vat.read');
SELECT grant_tenant_role_permission('intern', 'tenant.vat.read');
SELECT grant_tenant_role_permission('admin', 'tenant.vat.write');
SELECT grant_tenant_role_permission('accountant', 'tenant.vat.write');
SELECT grant_tenant_role_permission('member', 'tenant.vat.write');

-- Banka izinleri (#19): ekstre yükleme ve banka hareketleri.
-- Kural (000044, DATABASE_RULES.md): sahip yeni tenant.* iznini kendiliğinden
-- alır; diğer rollere grant_tenant_role_permission ile şablon ve ofis
-- kopyalarına birlikte verilir. Stajyer yalnızca okur.
INSERT INTO permissions (name, slug) VALUES
    ('Read tenant bank data', 'tenant.bank.read'),
    ('Write tenant bank data', 'tenant.bank.write')
ON CONFLICT (slug) DO NOTHING;

SELECT grant_tenant_role_permission(r, p)
FROM unnest(ARRAY['admin', 'accountant', 'member']) AS r,
     unnest(ARRAY['tenant.bank.read', 'tenant.bank.write']) AS p;

SELECT grant_tenant_role_permission('intern', 'tenant.bank.read');

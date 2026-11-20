-- Sorumlu müşavir son onayı (#566): taslak fişi hazırlamak (write) ile
-- onaylamak (approve) ayrılır. Personel taslak hazırlar; onay sahip,
-- yönetici ve mali müşavirde. Sahip yeni tenant.* iznini kendiliğinden
-- alır (trg_permissions_grant_tenant_owner, 000044).
--
-- roles ve role_permissions FORCE altında (000086): izin ofis rollerine
-- ancak sistem kipinde yazılır (DATABASE_RULES).
SET LOCAL app.system = 'on';

INSERT INTO permissions (name, slug) VALUES
    ('Approve journal drafts (final check)', 'tenant.journals.approve')
ON CONFLICT (slug) DO NOTHING;

SELECT grant_tenant_role_permission(r, 'tenant.journals.approve')
FROM unnest(ARRAY['admin', 'accountant']) AS r;

-- Rol tanımlarına uyum (000044): mali müşavir personelin bütün izinlerine
-- sahiptir, stajyer personelin okuma izinlerine. Sonraki migration'lar yeni
-- izinleri yalnızca personele verdiği için mali müşavir Fişler'i bile
-- göremiyordu. Bordro ve beyan okuma stajyere bilerek verilmez (maaş verisi,
-- 000102 / 000114).
SELECT grant_tenant_role_permission('accountant', p)
FROM unnest(ARRAY[
    'tenant.excel_imports.read', 'tenant.excel_imports.write',
    'tenant.journals.read', 'tenant.journals.write',
    'tenant.privacy.request',
    'tenant.receipts.read', 'tenant.receipts.write',
    'tenant.review.read', 'tenant.review.decide',
    'tenant.summaries.read', 'tenant.summaries.write'
]) AS p;

SELECT grant_tenant_role_permission('intern', p)
FROM unnest(ARRAY[
    'tenant.excel_imports.read', 'tenant.journals.read', 'tenant.receipts.read',
    'tenant.review.read', 'tenant.summaries.read'
]) AS p;

-- Ofisin kendi tanımladığı (sistem dışı) rollerden fiş yazabilenler bugün
-- onaylayabiliyordu; onayı kaybetmesinler.
INSERT INTO role_permissions (role_id, permission_id)
SELECT rp.role_id, p.id
FROM role_permissions rp
JOIN roles r ON r.id = rp.role_id
JOIN permissions w ON w.id = rp.permission_id AND w.slug = 'tenant.journals.write'
JOIN permissions p ON p.slug = 'tenant.journals.approve'
WHERE r.scope = 'tenant' AND r.organization_id IS NOT NULL AND NOT r.is_system
ON CONFLICT DO NOTHING;

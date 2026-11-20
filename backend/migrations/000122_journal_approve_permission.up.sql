-- Sorumlu müşavir son onayı (#566): taslak fişi hazırlamak (write) ile
-- onaylamak (approve) ayrılır. Stajyer/personel (member) taslak hazırlar;
-- onay sahip ve yöneticide. Kurum içi özel rollerden write'ı olanlara da
-- verilir, böylece bugün onaylayan bir rol onay yetkisini kaybetmez.
INSERT INTO permissions (name, slug) VALUES
    ('Approve journal drafts (final check)', 'tenant.journals.approve')
ON CONFLICT (slug) DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM roles r
JOIN permissions p ON p.slug = 'tenant.journals.approve'
WHERE r.slug IN ('owner', 'admin') AND r.scope = 'tenant' AND r.organization_id IS NULL
ON CONFLICT DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id)
SELECT rp.role_id, p.id
FROM role_permissions rp
JOIN roles r ON r.id = rp.role_id
JOIN permissions w ON w.id = rp.permission_id AND w.slug = 'tenant.journals.write'
JOIN permissions p ON p.slug = 'tenant.journals.approve'
WHERE r.scope = 'tenant' AND r.organization_id IS NOT NULL
ON CONFLICT DO NOTHING;

-- Yevmiye taslağı ve muhasebe programı çıktısı uçları (issue #143, #26).
-- read: taslak ve çıktı listesi, dosya indirme. write: taslak oluşturma,
-- onay. export: Luca/Zirve/Excel çıktısı alma ve çıktıdan geri alma.
INSERT INTO permissions (name, slug) VALUES
    ('Read journal drafts and exports', 'tenant.journals.read'),
    ('Write and approve journal drafts', 'tenant.journals.write'),
    ('Export journal drafts to accounting programs', 'tenant.journals.export')
ON CONFLICT (slug) DO NOTHING;

-- owner/admin: hepsi. member: taslak hazırlar ve görür; çıktı ve geri alma yöneticide.
INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM roles r
JOIN permissions p ON p.slug IN ('tenant.journals.read', 'tenant.journals.write', 'tenant.journals.export')
WHERE r.slug IN ('owner', 'admin') AND r.scope = 'tenant' AND r.organization_id IS NULL
ON CONFLICT DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM roles r
JOIN permissions p ON p.slug IN ('tenant.journals.read', 'tenant.journals.write')
WHERE r.slug = 'member' AND r.scope = 'tenant' AND r.organization_id IS NULL
ON CONFLICT DO NOTHING;

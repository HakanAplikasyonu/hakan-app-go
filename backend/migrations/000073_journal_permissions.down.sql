DELETE FROM role_permissions
WHERE permission_id IN (SELECT id FROM permissions WHERE slug IN
    ('tenant.journals.read', 'tenant.journals.write', 'tenant.journals.export'));
DELETE FROM permissions WHERE slug IN
    ('tenant.journals.read', 'tenant.journals.write', 'tenant.journals.export');

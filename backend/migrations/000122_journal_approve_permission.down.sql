DELETE FROM role_permissions
WHERE permission_id IN (SELECT id FROM permissions WHERE slug = 'tenant.journals.approve');
DELETE FROM permissions WHERE slug = 'tenant.journals.approve';

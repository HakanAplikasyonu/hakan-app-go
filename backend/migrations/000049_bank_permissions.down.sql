DELETE FROM role_permissions
WHERE permission_id IN (SELECT id FROM permissions WHERE slug IN ('tenant.bank.read', 'tenant.bank.write'));
DELETE FROM permissions WHERE slug IN ('tenant.bank.read', 'tenant.bank.write');

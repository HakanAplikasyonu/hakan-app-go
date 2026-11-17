SET LOCAL app.system = 'on';

DROP TABLE IF EXISTS taxpayer_documents;

DELETE FROM role_permissions
WHERE permission_id IN (SELECT id FROM permissions WHERE slug IN ('tenant.documents.read', 'tenant.documents.write'));
DELETE FROM permissions WHERE slug IN ('tenant.documents.read', 'tenant.documents.write');

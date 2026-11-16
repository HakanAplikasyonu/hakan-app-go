DELETE FROM role_permissions
WHERE permission_id IN (SELECT id FROM permissions WHERE slug IN ('tenant.legal.accept', 'platform.legal.manage'));
DELETE FROM permissions WHERE slug IN ('tenant.legal.accept', 'platform.legal.manage');
DROP TABLE IF EXISTS legal_acceptances;
DROP TABLE IF EXISTS legal_documents;
DROP FUNCTION IF EXISTS legal_documents_freeze_published();

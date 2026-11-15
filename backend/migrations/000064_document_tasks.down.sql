-- Eksik belge görevlerini kaldırır: görevler, yüklemeler ve hareket ↔ belge
-- eşleşmeleri silinir (attachment satırları ve dosyalar kalır).
DELETE FROM notification_templates WHERE code IN ('doc_tasks.created', 'doc_tasks.reminder');
DELETE FROM role_permissions
WHERE permission_id IN (SELECT id FROM permissions WHERE slug IN ('tenant.doc_tasks.read', 'tenant.doc_tasks.write'));
DELETE FROM permissions WHERE slug IN ('tenant.doc_tasks.read', 'tenant.doc_tasks.write');
DROP TABLE IF EXISTS document_task_uploads;
DROP TABLE IF EXISTS document_tasks;
DROP TABLE IF EXISTS bank_transaction_documents;

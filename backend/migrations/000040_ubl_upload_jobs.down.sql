DELETE FROM role_permissions
WHERE permission_id IN (SELECT id FROM permissions WHERE slug IN ('tenant.invoices.read', 'tenant.invoices.write'));
DELETE FROM permissions WHERE slug IN ('tenant.invoices.read', 'tenant.invoices.write');
-- Veri kaybı: yükleme işleri ve raporları silinir (faturalar kalır).
DROP TABLE IF EXISTS ubl_upload_jobs;

-- Fiş yakalama tablolarını kaldırır: masraf ve Z raporu kayıtları, araç listesi ve yüklemeler silinir.
DELETE FROM role_permissions
WHERE permission_id IN (SELECT id FROM permissions WHERE slug IN
    ('tenant.receipts.read', 'tenant.receipts.write', 'tenant.receipts.approve'));
DELETE FROM permissions WHERE slug IN ('tenant.receipts.read', 'tenant.receipts.write', 'tenant.receipts.approve');
DROP TABLE IF EXISTS receipt_captures;
DROP TABLE IF EXISTS retail_sales;
DROP TABLE IF EXISTS expenses;
DROP TABLE IF EXISTS taxpayer_vehicles;

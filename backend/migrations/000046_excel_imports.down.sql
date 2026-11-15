-- Excel içe aktarma tablolarını kaldırır (yüklemeler, satır sonuçları ve şablonlar silinir;
-- yazılmış faturalar kalır).
DELETE FROM role_permissions
WHERE permission_id IN (SELECT id FROM permissions WHERE slug IN
    ('tenant.excel_imports.read', 'tenant.excel_imports.write', 'tenant.excel_imports.commit'));
DELETE FROM permissions WHERE slug IN
    ('tenant.excel_imports.read', 'tenant.excel_imports.write', 'tenant.excel_imports.commit');
DROP TABLE IF EXISTS excel_import_rows;
DROP TABLE IF EXISTS excel_imports;
DROP TABLE IF EXISTS excel_import_templates;

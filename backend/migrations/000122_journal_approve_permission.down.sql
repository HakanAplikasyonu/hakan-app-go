SET LOCAL app.system = 'on';

DELETE FROM role_permissions
WHERE permission_id IN (SELECT id FROM permissions WHERE slug = 'tenant.journals.approve');
DELETE FROM permissions WHERE slug = 'tenant.journals.approve';

-- Mali müşavir ve stajyer rol uyumu geri alınır (şablon + sistem kopyaları).
DELETE FROM role_permissions rp
USING roles r, permissions p
WHERE r.id = rp.role_id AND p.id = rp.permission_id
  AND r.scope = 'tenant' AND (r.organization_id IS NULL OR r.is_system)
  AND ((r.slug = 'accountant' AND p.slug IN (
        'tenant.excel_imports.read', 'tenant.excel_imports.write',
        'tenant.journals.read', 'tenant.journals.write',
        'tenant.privacy.request',
        'tenant.receipts.read', 'tenant.receipts.write',
        'tenant.review.read', 'tenant.review.decide',
        'tenant.summaries.read', 'tenant.summaries.write'))
    OR (r.slug = 'intern' AND p.slug IN (
        'tenant.excel_imports.read', 'tenant.journals.read', 'tenant.receipts.read',
        'tenant.review.read', 'tenant.summaries.read')));

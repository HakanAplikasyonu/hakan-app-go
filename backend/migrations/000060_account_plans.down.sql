DELETE FROM role_permissions
WHERE permission_id IN (SELECT id FROM permissions WHERE slug IN ('tenant.account_plan.read', 'tenant.account_plan.write'));
DELETE FROM permissions WHERE slug IN ('tenant.account_plan.read', 'tenant.account_plan.write');
DROP TABLE IF EXISTS account_plan_accounts;
DROP TABLE IF EXISTS account_plan_imports;

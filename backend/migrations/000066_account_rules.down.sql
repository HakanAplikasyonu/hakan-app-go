DELETE FROM role_permissions
WHERE permission_id IN (SELECT id FROM permissions WHERE slug IN ('tenant.account_rules.read', 'tenant.account_rules.write'));
DELETE FROM permissions WHERE slug IN ('tenant.account_rules.read', 'tenant.account_rules.write');
DROP INDEX IF EXISTS idx_suggestions_account_memory;
DROP TABLE IF EXISTS account_assignments;
DROP TRIGGER IF EXISTS trg_engagements_copy_account_rules ON engagements;
DROP FUNCTION IF EXISTS engagements_copy_account_rules();
DROP FUNCTION IF EXISTS copy_office_account_rules(BIGINT, BIGINT);
DROP TABLE IF EXISTS account_rules;
DROP FUNCTION IF EXISTS account_rules_immutable();

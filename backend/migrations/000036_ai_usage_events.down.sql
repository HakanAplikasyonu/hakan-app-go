-- Drops the AI cost ledger (all recorded usage is lost).
DELETE FROM role_permissions
WHERE permission_id IN (SELECT id FROM permissions WHERE slug = 'tenant.ai.usage.read');
DELETE FROM permissions WHERE slug = 'tenant.ai.usage.read';
DROP TABLE IF EXISTS ai_usage_events;

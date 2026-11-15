DELETE FROM notification_templates WHERE code = 'agents.offline';
DELETE FROM role_permissions
WHERE permission_id IN (SELECT id FROM permissions WHERE slug IN ('tenant.agents.read', 'tenant.agents.write'));
DELETE FROM permissions WHERE slug IN ('tenant.agents.read', 'tenant.agents.write');
DROP TABLE IF EXISTS desktop_agents;

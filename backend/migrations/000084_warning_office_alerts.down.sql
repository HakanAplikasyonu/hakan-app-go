DELETE FROM notification_templates WHERE code = 'warnings.blocker';
DROP INDEX IF EXISTS idx_data_warnings_open_blockers;
DROP TABLE IF EXISTS warning_office_alerts;

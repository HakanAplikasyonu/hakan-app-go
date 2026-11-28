-- Sahip FORCE altında: veri silen adım sistem kipinde (DATABASE_RULES).
SET LOCAL app.system = 'on';

-- Yardımcıyla açılmış yevmiye aktarımları silinir (yöntem geri gelmez).
DELETE FROM accounting_transfers WHERE method = 'luca_web';
ALTER TABLE accounting_transfers DROP CONSTRAINT chk_accounting_transfers_method;
ALTER TABLE accounting_transfers ADD CONSTRAINT chk_accounting_transfers_method
    CHECK (method IN ('agent_folder', 'manual'));

DROP TABLE IF EXISTS agent_task_files;
DROP TABLE IF EXISTS agent_task_counterparties;
DROP TABLE IF EXISTS agent_tasks;
DROP TABLE IF EXISTS desktop_helper_settings;

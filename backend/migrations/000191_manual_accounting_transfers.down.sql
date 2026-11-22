DELETE FROM accounting_transfers WHERE method = 'manual';
ALTER TABLE accounting_transfers DROP CONSTRAINT chk_accounting_transfers_method;
ALTER TABLE accounting_transfers ADD CONSTRAINT chk_accounting_transfers_method
    CHECK (method IN ('agent_folder'));

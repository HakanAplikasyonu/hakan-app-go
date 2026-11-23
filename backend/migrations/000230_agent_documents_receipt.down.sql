-- Sahip FORCE altında: veri değiştiren adımlar sistem kipinde (DATABASE_RULES).
SET LOCAL app.system = 'on';

UPDATE agent_documents SET kind = 'unknown' WHERE kind = 'receipt';
ALTER TABLE agent_documents DROP CONSTRAINT chk_agent_documents_kind;
ALTER TABLE agent_documents ADD CONSTRAINT chk_agent_documents_kind
    CHECK (kind IN ('unknown', 'payroll', 'trial_balance', 'account_plan', 'foreign_invoice'));

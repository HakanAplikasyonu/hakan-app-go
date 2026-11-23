-- Yerel ajanın izlediği klasöre bırakılan fiş / fatura / Z raporu
-- fotoğrafı (JPEG, PNG, HEIC): mobil ve WhatsApp ile aynı fiş okumasına
-- gider. Mükellefi bulunamayan fotoğraf "eşleştir" kaydı olarak bekler.
ALTER TABLE agent_documents DROP CONSTRAINT chk_agent_documents_kind;
ALTER TABLE agent_documents ADD CONSTRAINT chk_agent_documents_kind
    CHECK (kind IN ('unknown', 'payroll', 'trial_balance', 'account_plan', 'foreign_invoice', 'receipt'));

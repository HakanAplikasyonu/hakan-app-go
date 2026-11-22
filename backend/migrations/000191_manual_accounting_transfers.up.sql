-- "Luca'ya yükledim" (#780 ay sonu kapanış): masaüstü ajanı olmayan ofis
-- çıktı dosyasını indirip programa elle yükler. Bu yükleme, çıktının
-- aktarım görevi olarak kaydedilir: method = 'manual', doğrudan 'imported',
-- kim (requested_by / completed_by) ve ne zaman (completed_at) ile. Aynı
-- çıktı için tek açık ya da aktarılmış görev kuralı
-- (uq_accounting_transfers_live) elle yükleme için de geçerlidir.
ALTER TABLE accounting_transfers DROP CONSTRAINT chk_accounting_transfers_method;
ALTER TABLE accounting_transfers ADD CONSTRAINT chk_accounting_transfers_method
    CHECK (method IN ('agent_folder', 'manual'));

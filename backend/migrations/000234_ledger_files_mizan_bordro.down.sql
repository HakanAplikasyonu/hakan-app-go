-- Mizan ve bordro dosyaları "diğer" olur; eski tür listesi geri gelir.
UPDATE ledger_files SET kind = 'other' WHERE kind IN ('mizan', 'bordro');
ALTER TABLE ledger_files DROP CONSTRAINT chk_ledger_files_kind;
ALTER TABLE ledger_files ADD CONSTRAINT chk_ledger_files_kind
    CHECK (kind IN ('isletme_defteri', 'e_defter_yevmiye', 'e_defter_kebir', 'berat', 'other'));

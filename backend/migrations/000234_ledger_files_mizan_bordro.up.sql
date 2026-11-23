-- Defterler (#780, Oğuz'un geri bildirimi): Luca'dan alınan mizan ve ücret
-- bordrosu dökümleri de mükellefin defter dosyalarına yüklenir; özetleri
-- (mizan toplamları, bordro alt özeti) ledger_files.summary'de durur.
ALTER TABLE ledger_files DROP CONSTRAINT chk_ledger_files_kind;
ALTER TABLE ledger_files ADD CONSTRAINT chk_ledger_files_kind
    CHECK (kind IN ('isletme_defteri', 'e_defter_yevmiye', 'e_defter_kebir', 'berat', 'mizan', 'bordro', 'other'));

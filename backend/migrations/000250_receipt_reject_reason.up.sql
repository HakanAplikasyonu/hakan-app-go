-- Fiş ret gerekçesi (#780): ofis mükellefin gönderdiği fişi, masrafı ya da
-- Z raporunu reddederken bir gerekçe seçer (okunaksız, eksik bilgi, kişisel
-- harcama, mükerrer, yanlış mükellef, diğer) ve isterse açıklama yazar.
-- Mükellef bunu Gönderdiklerim ekranında görür. Üç karar yolu da (fiş onayı,
-- Masraflar, Z raporları) kararı fiş yakalama kaydına (receipt_captures)
-- yazar ve Gönderdiklerim buradan okur; gerekçe de yalnız burada tutulur.
-- Denetim kaydı (activity_events) gerekçeyi ayrıca saklar.
-- receipt_captures tablosunun mevcut RLS'i (rls_apply_engagement) yeni
-- sütunları da kapsar; ek politika gerekmez.
ALTER TABLE receipt_captures
    ADD COLUMN reject_reason_code TEXT NULL,
    ADD COLUMN reject_reason_text TEXT NULL,
    ADD CONSTRAINT chk_receipt_captures_reject_reason_code
        CHECK (reject_reason_code IS NULL OR reject_reason_code IN
               ('okunaksiz', 'eksik_bilgi', 'kisisel_harcama', 'mukerrer', 'yanlis_mukellef', 'diger')),
    ADD CONSTRAINT chk_receipt_captures_reject_reason_text
        CHECK (reject_reason_text IS NULL OR char_length(reject_reason_text) BETWEEN 1 AND 500);

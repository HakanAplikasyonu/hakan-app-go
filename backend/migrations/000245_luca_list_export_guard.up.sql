-- Luca'ya aktar (Excel) listeleri (Oğuz'un geri bildirimi): gelen / giden
-- fatura, banka hareketi ve Z raporu da Masraflar ekranındaki gibi Luca fiş
-- dosyası olarak iner. Çift aktarım koruması masrafınkiyle (000242) aynı
-- tabloda tutulur: expense_export_documents artık her liste belgesini
-- taşır (source_kind + source_uuid). ListLiveDocumentExports bu tabloyu
-- zaten okuduğundan bir fatura (anahtar: ETTN ya da "inv:<uuid>") hangi
-- yoldan gitmiş olursa olsun (yevmiye dışa aktarımı ya da liste dosyası)
-- ikinci çıktıda varsayılan olarak dışarıda kalır.
ALTER TABLE expense_export_documents ALTER COLUMN expense_uuid DROP NOT NULL;
ALTER TABLE expense_export_documents ADD COLUMN source_kind TEXT NOT NULL DEFAULT 'expense';
-- Fatura / banka hareketi / Z raporu mükellef verisidir; yalnız uuid'si
-- tutulur, yabancı anahtar yok (expense_uuid gibi).
ALTER TABLE expense_export_documents ADD COLUMN source_uuid UUID NULL;
ALTER TABLE expense_export_documents ADD CONSTRAINT chk_expense_export_documents_source CHECK (
    source_kind IN ('expense', 'invoice', 'bank_transaction', 'retail_sale')
    AND (source_kind <> 'expense' OR expense_uuid IS NOT NULL)
    AND (source_kind = 'expense' OR source_uuid IS NOT NULL)
);

-- Çift aktarım koruması banka hareketi ve Z raporunda da iki yönlü
-- (Luca denetimi). Luca'ya aktar (Excel) listeleri (000245) giden banka
-- hareketini "bank:<uuid>", Z raporunu "z:<uuid>" anahtarıyla
-- expense_export_documents'a yazıyor; yevmiye dışa aktarımı (fiş dosyası)
-- bu belgeleri tanımıyordu. Bir Z raporu ya da banka hareketi önce liste
-- dosyasıyla, sonra onaylı yevmiye fişiyle (ya da tersi) uyarısız ikinci
-- kez Luca'ya gidebiliyordu.
--
-- Artık yevmiye dışa aktarımı banka fişi satırının hareketini ve Z raporu
-- fişinin satışını belge olarak tanır, aynı anahtarla
-- export_batch_documents'a yazar; ListLiveDocumentExports iki tabloyu
-- birlikte okuduğu için hangi yoldan gitmiş olursa olsun ikinci çıktıda
-- belge varsayılan olarak dışarıda kalır.
ALTER TABLE export_batch_documents DROP CONSTRAINT chk_export_batch_documents_kind;
ALTER TABLE export_batch_documents ADD CONSTRAINT chk_export_batch_documents_kind
    CHECK (source_kind IN ('invoice', 'expense', 'bank_transaction', 'retail_sale'));

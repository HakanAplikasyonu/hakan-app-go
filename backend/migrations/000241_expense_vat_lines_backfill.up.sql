-- Masraflarda KDV oran kırılımı (#780): Easytrade'den gelmiş masrafların
-- kırılımı. Easytrade masraf detayında oran satırı yok, masrafın tek bir
-- açık oranı var (tax_rate); o oranla gelmiş masraf tek oranlı kırılımdır
-- (matrah = tutar − KDV). Oranı gelmemiş masrafa satır yazılmaz: ekranda
-- "kırılım yok" görünür, toplam KDV'den oran türetilmez.
--
-- Bundan sonra kırılımı aktarım yazar; bu adım yalnızca eski kayıtlar için.
-- Sahip FORCE altında: veri yazan adım sistem kipinde (DATABASE_RULES).
SET LOCAL app.system = 'on';

INSERT INTO expense_vat_lines (taxpayer_id, expense_id, tax_rate, tax_base, tax_amount)
SELECT e.taxpayer_id, e.id, e.tax_rate, e.amount - e.tax_amount, e.tax_amount
FROM expenses e
WHERE e.source_system = 'easytrade' AND e.capture_method = 'api'
  AND e.tax_rate IS NOT NULL AND e.amount IS NOT NULL AND e.tax_amount IS NOT NULL
  AND e.tax_amount <= e.amount
  AND NOT EXISTS (SELECT 1 FROM expense_vat_lines l WHERE l.taxpayer_id = e.taxpayer_id AND l.expense_id = e.id)
ON CONFLICT (taxpayer_id, expense_id, tax_rate) DO NOTHING;

-- Ofis / mükellef masraf kategorileri ve fişlerin hesap kodu seçimleri
-- silinir. Yevmiye taslakları (journal_drafts) ve kaynak masraflar kalır.
DROP INDEX IF EXISTS idx_expenses_list;
DROP TABLE IF EXISTS receipt_postings;
DROP TABLE IF EXISTS expense_categories;

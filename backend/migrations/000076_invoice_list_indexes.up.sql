-- Sorgu performansı (#182): fatura listeleri (ofis ve mükellef paneli)
-- issue_date DESC, id DESC sırasıyla ve imleçle sayfalanır. Bu indeks sırayı
-- hazır verir (sıralama yok, LIMIT erken durur) ve imleç koşulunu
-- (issue_date, id) < (...) indeks sınır koşulu yapar.
CREATE INDEX IF NOT EXISTS idx_invoices_taxpayer_issue
    ON invoices (taxpayer_id, issue_date DESC, id DESC);

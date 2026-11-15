-- Veri kaybı: ortak veri modelindeki bütün kaynak veriler (bağlantılar, cariler,
-- faturalar, ekler, senkron durumu) ve ofis uyarı / değişiklik kayıtları silinir.
DROP TABLE IF EXISTS change_requests;
DROP TABLE IF EXISTS data_warnings;
DROP TABLE IF EXISTS sync_runs;
DROP TABLE IF EXISTS sync_windows;
DROP TABLE IF EXISTS invoice_tax_summaries;
DROP TABLE IF EXISTS invoice_items;
DROP TABLE IF EXISTS invoices;
DROP TABLE IF EXISTS attachments;
DROP TABLE IF EXISTS customer_source_links;
DROP TABLE IF EXISTS customers;
DROP TABLE IF EXISTS source_connections;

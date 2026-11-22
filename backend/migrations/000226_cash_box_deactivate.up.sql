-- Müşavir mükellefin banka hesabını kapatabilir (#780): hesap silinmez
-- (hareketleri ve fişleri bağlı), pasife alınır. NULL: hesap kullanımda.
ALTER TABLE cash_boxes ADD COLUMN deactivated_at TIMESTAMPTZ NULL;

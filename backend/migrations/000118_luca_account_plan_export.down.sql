-- Üretilen alt hesap kodları ve indirme geçmişi silinir. Kodlar Luca'ya
-- aktarılmışsa portaldaki eşleme kaybolur; counterparty_accounts'a yazılan
-- cari hafızası kalır.
DROP TABLE IF EXISTS account_plan_exports;
DROP TABLE IF EXISTS generated_subaccounts;

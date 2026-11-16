-- RLS zorlama, 2. grup (issue #166): engagement ayarları.
--   vat_carry_forwards  KDV devri açılışları (vatforecast; yalnızca API)
--   export_settings     yevmiye çıktı ayarları (journals; yalnızca API)
-- İki tablonun bütün erişim yolları rls.Middleware ile kapsamlı istek
-- işlemi içinde çalışıyor.
SELECT rls_enforce('vat_carry_forwards'::regclass);
SELECT rls_enforce('export_settings'::regclass);

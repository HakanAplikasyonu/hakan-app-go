-- Politikaların içinden okunan SECURITY DEFINER fonksiyonları (issue #312).
--
-- app_engagement_visible (000046) ve engagement_document_visible (000058)
-- tablo sahibi olarak çalışır ve engagement_assignments, engagements,
-- taxpayer_history_shares okur. Sahip süper kullanıcıyken RLS'yi atlıyordu.
-- 000086 bu tablolarda FORCE açtı: süper kullanıcı olmayan sahip (yönetilen
-- Postgres, scripts/db-roles.sql) artık yalnızca app.system açıkken satır
-- görür. Fonksiyonlar bu yüzden hiçbir atama görmüyor, atanmış personel
-- engagement'ına erişemiyordu (TestRLSEngagementScope, süper kullanıcı
-- olmayan sahiple).
--
-- Her fonksiyon kendi süresince app.system'i açar: sahibin rls_system
-- politikası fonksiyonun içindeki okumalara uygulanır, fonksiyondan çıkınca
-- ayar eski değerine döner. Fonksiyonların içinde çağıranın kodu çalışmaz.
ALTER FUNCTION app_engagement_visible(BIGINT, BIGINT) SET app.system = 'on';
ALTER FUNCTION engagement_document_visible(BIGINT, DATE) SET app.system = 'on';

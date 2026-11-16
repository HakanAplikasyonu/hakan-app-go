-- Veritabanı rolleri (issue #312). Bir kez, bir süper kullanıcıyla (ya da
-- yönetilen Postgres'in yönetici rolüyle) çalıştırılır; tekrar çalıştırmak
-- güvenlidir.
--
--   psql "$ADMIN_URL" -v ON_ERROR_STOP=1 \
--        -v owner_password="$DB_OWNER_PASSWORD" \
--        -v backup_password="$DB_BACKUP_PASSWORD" \
--        -f scripts/db-roles.sql
--
-- Roller:
--   hakan_owner        tabloların ve migration'ların sahibi. Süper kullanıcı
--                      değil, BYPASSRLS değil: FORCE ROW LEVEL SECURITY
--                      (000086) ona da uygulanır, tablolara yalnızca
--                      rls.System (app.system) içinden erişir. Uygulama ve
--                      migrate bu rolle bağlanır (DB_USER).
--   hakan_rls_runtime  uygulama havuzunun her bağlantıda geçtiği rol
--   hakan_rls_app      (000046, 000074). NOLOGIN; sahip, migration'ların
--   hakan_rls_worker   rolleri kendine verebilmesi için ADMIN OPTION ile üye.
--   hakan_backup       pg_dump: yalnızca okur (pg_read_all_data), RLS'yi
--                      atlar (BYPASSRLS). Yazamaz, tablo sahibi değildir.
--
-- Parola değişkenleri verilmezse rollerin parolası değiştirilmez.

DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'hakan_owner') THEN
        CREATE ROLE hakan_owner LOGIN NOSUPERUSER NOBYPASSRLS CREATEDB NOCREATEROLE;
    END IF;
    ALTER ROLE hakan_owner LOGIN NOSUPERUSER NOBYPASSRLS CREATEDB NOCREATEROLE;

    IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'hakan_backup') THEN
        CREATE ROLE hakan_backup LOGIN NOSUPERUSER BYPASSRLS NOCREATEDB NOCREATEROLE;
    END IF;
    ALTER ROLE hakan_backup LOGIN NOSUPERUSER BYPASSRLS NOCREATEDB NOCREATEROLE;

    IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'hakan_rls_runtime') THEN
        CREATE ROLE hakan_rls_runtime NOLOGIN NOBYPASSRLS;
    END IF;
    IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'hakan_rls_app') THEN
        CREATE ROLE hakan_rls_app NOLOGIN NOBYPASSRLS;
    END IF;
    IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'hakan_rls_worker') THEN
        CREATE ROLE hakan_rls_worker NOLOGIN NOBYPASSRLS;
    END IF;
END
$$;

-- Sahip, uygulama rollerine geçebilir (SET ROLE) ve migration'lardaki
-- "GRANT ... TO current_user" satırlarını çalıştırabilir.
GRANT hakan_rls_runtime, hakan_rls_app, hakan_rls_worker TO hakan_owner WITH ADMIN TRUE, INHERIT FALSE, SET TRUE;

-- Politikaların içinden okunan SECURITY DEFINER fonksiyonları kendi
-- süreleri için app.system'i açar (migration 000089). Süper kullanıcı
-- olmayan sahip bu ayarı fonksiyona yazabilsin (PostgreSQL 15+).
GRANT SET ON PARAMETER app.system TO hakan_owner;

-- Yedek rolü: bütün tabloları ve dizileri okur (PostgreSQL 14+).
GRANT pg_read_all_data TO hakan_backup;

\if :{?owner_password}
ALTER ROLE hakan_owner PASSWORD :'owner_password';
\endif
\if :{?backup_password}
ALTER ROLE hakan_backup PASSWORD :'backup_password';
\endif

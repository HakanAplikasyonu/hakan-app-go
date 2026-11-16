-- Veritabanı sahibini değiştirir (issue #317): scripts/db-owner-migrate.sh
-- (app -> hakan_owner) ve scripts/db-owner-rollback.sh (hakan_owner -> app)
-- bu dosyayı çalıştırır. Tek işlem: bir adım ya da doğrulama düşerse hiçbir
-- şey değişmez. Tekrar çalıştırmak güvenlidir (taşınacak bir şey kalmamışsa
-- yalnızca doğrulama çalışır).
--
-- psql değişkenleri:
--   from  şimdiki sahip (rol)
--   to    yeni sahip (rol, var olmalı: scripts/db-roles.sql)
--   dry   on: aynı işlemi yap, doğrula, listele ve GERİ AL (--dry-run)
--
-- Neler değişir (yalnızca bu veritabanında, public şemasında):
--   - veritabanının ve public şemasının sahibi;
--   - tablo, dizi, görünüm, fonksiyon ve tiplerin sahibi (REASSIGN OWNED'ın
--     bu veritabanıyla sınırlı karşılığı: REASSIGN OWNED başka
--     veritabanlarının sahipliğini de taşırdı);
--   - `from` rolüne yazılmış politikalar (rls_system: rls_enforce /
--     rls_owner_force onları TO CURRENT_USER ile oluşturdu). REASSIGN OWNED
--     politika rollerini değiştirmez; değişmeseydi rls.System yeni sahiple
--     FORCE'lu tablolarda hiçbir şey görmezdi;
--   - varsayılan yetkiler (000046 / 000074 ALTER DEFAULT PRIVILEGES): `from`
--     için tanımlıydı; yeni sahibin oluşturacağı tablolar da uygulama
--     rollerine açılsın diye `to` için yeniden tanımlanır.
-- Uzantılar (pgcrypto, citext) taşınmaz: sahiplikleri değiştirilemez ve
-- yalnızca DROP EXTENSION (down -all) için gerekir.
\set ON_ERROR_STOP on
\if :{?from}
\else
\echo 'db-owner.sql: -v from=ROL gerekli'
\quit
\endif
\if :{?to}
\else
\echo 'db-owner.sql: -v to=ROL gerekli'
\quit
\endif
\if :{?dry}
\else
\set dry off
\endif

BEGIN;
-- Rolleri aynı işlemde kur (yalnızca geçişte; geri dönüş rollere dokunmaz).
\if :{?roles_file}
\i :roles_file
\endif
SELECT set_config('owner_move.from', :'from', true),
       set_config('owner_move.to', :'to', true) \gset noop_

DO $$
DECLARE
    v_from TEXT := current_setting('owner_move.from');
    v_to   TEXT := current_setting('owner_move.to');
    r      RECORD;
    n      INT := 0;
    stmt   TEXT;
    roles  TEXT;
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = v_from) THEN
        RAISE EXCEPTION 'db-owner: from role % does not exist', v_from;
    END IF;
    IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = v_to) THEN
        RAISE EXCEPTION 'db-owner: to role % does not exist (run scripts/db-roles.sql)', v_to;
    END IF;

    IF (SELECT pg_get_userbyid(datdba) FROM pg_database WHERE datname = current_database()) = v_from THEN
        stmt := format('ALTER DATABASE %I OWNER TO %I', current_database(), v_to);
        RAISE NOTICE '%', stmt; EXECUTE stmt; n := n + 1;
    END IF;
    IF (SELECT pg_get_userbyid(nspowner) FROM pg_namespace WHERE nspname = 'public') = v_from THEN
        stmt := format('ALTER SCHEMA public OWNER TO %I', v_to);
        RAISE NOTICE '%', stmt; EXECUTE stmt; n := n + 1;
    END IF;

    -- Tables first: ALTER TABLE also moves their identity / serial sequences.
    FOR r IN
        SELECT c.oid::regclass AS obj, c.relkind
        FROM pg_class c JOIN pg_namespace s ON s.oid = c.relnamespace
        WHERE s.nspname = 'public' AND pg_get_userbyid(c.relowner) = v_from
          AND c.relkind IN ('r', 'p', 'v', 'm', 'f', 'S')
        ORDER BY CASE c.relkind WHEN 'S' THEN 1 ELSE 0 END, c.relname
    LOOP
        -- An owned sequence moved with its table in this loop already.
        CONTINUE WHEN r.relkind = 'S' AND pg_get_userbyid((SELECT relowner FROM pg_class WHERE oid = r.obj)) <> v_from;
        stmt := format('ALTER %s %s OWNER TO %I',
            CASE r.relkind WHEN 'S' THEN 'SEQUENCE' WHEN 'v' THEN 'VIEW' WHEN 'm' THEN 'MATERIALIZED VIEW'
                           WHEN 'f' THEN 'FOREIGN TABLE' ELSE 'TABLE' END, r.obj, v_to);
        RAISE NOTICE '%', stmt; EXECUTE stmt; n := n + 1;
    END LOOP;

    FOR r IN
        SELECT p.oid::regprocedure AS obj, p.prokind
        FROM pg_proc p JOIN pg_namespace s ON s.oid = p.pronamespace
        WHERE s.nspname = 'public' AND pg_get_userbyid(p.proowner) = v_from
          AND NOT EXISTS (SELECT 1 FROM pg_depend d WHERE d.objid = p.oid AND d.deptype = 'e')
        ORDER BY 1::text
    LOOP
        stmt := format('ALTER %s %s OWNER TO %I',
            CASE r.prokind WHEN 'p' THEN 'PROCEDURE' WHEN 'a' THEN 'AGGREGATE' ELSE 'FUNCTION' END, r.obj, v_to);
        RAISE NOTICE '%', stmt; EXECUTE stmt; n := n + 1;
    END LOOP;

    -- Standalone types (enums, domains, composite types not tied to a table).
    FOR r IN
        SELECT t.oid::regtype AS obj, t.typtype
        FROM pg_type t JOIN pg_namespace s ON s.oid = t.typnamespace
        WHERE s.nspname = 'public' AND pg_get_userbyid(t.typowner) = v_from
          AND t.typelem = 0 AND (t.typrelid = 0 OR (SELECT relkind FROM pg_class WHERE oid = t.typrelid) = 'c')
          AND NOT EXISTS (SELECT 1 FROM pg_depend d WHERE d.objid = t.oid AND d.deptype = 'e')
    LOOP
        stmt := format('ALTER %s %s OWNER TO %I', CASE r.typtype WHEN 'd' THEN 'DOMAIN' ELSE 'TYPE' END, r.obj, v_to);
        RAISE NOTICE '%', stmt; EXECUTE stmt; n := n + 1;
    END LOOP;

    -- Policies written for the old owner (rls_system) follow the owner.
    FOR r IN
        SELECT p.schemaname, p.tablename, p.policyname, p.roles
        FROM pg_policies p
        WHERE p.schemaname = 'public' AND v_from = ANY (p.roles)
    LOOP
        SELECT string_agg(quote_ident(CASE WHEN x = v_from THEN v_to ELSE x END), ', ')
          INTO roles FROM unnest(r.roles) AS x;
        stmt := format('ALTER POLICY %I ON %I.%I TO %s', r.policyname, r.schemaname, r.tablename, roles);
        RAISE NOTICE '%', stmt; EXECUTE stmt; n := n + 1;
    END LOOP;

    -- Default privileges for objects the new owner creates (migrations).
    FOR stmt IN SELECT unnest(ARRAY[
        format('ALTER DEFAULT PRIVILEGES FOR ROLE %I IN SCHEMA public GRANT SELECT, INSERT, UPDATE, DELETE ON TABLES TO hakan_rls_app, hakan_rls_worker, hakan_rls_runtime', v_to),
        format('ALTER DEFAULT PRIVILEGES FOR ROLE %I IN SCHEMA public GRANT USAGE, SELECT ON SEQUENCES TO hakan_rls_app, hakan_rls_worker', v_to),
        format('ALTER DEFAULT PRIVILEGES FOR ROLE %I IN SCHEMA public GRANT USAGE, SELECT, UPDATE ON SEQUENCES TO hakan_rls_runtime', v_to)])
    LOOP
        EXECUTE stmt; -- idempotent; listed once below
    END LOOP;
    RAISE NOTICE 'db-owner: % object(s) moved from % to %; default privileges set for %', n, v_from, v_to, v_to;
END
$$;

-- Verification: any failure here rolls everything back.
DO $$
DECLARE
    v_from TEXT := current_setting('owner_move.from');
    v_to   TEXT := current_setting('owner_move.to');
    left_over TEXT;
    missing   TEXT;
BEGIN
    SELECT string_agg(c.relname, ', ') INTO left_over
    FROM pg_class c JOIN pg_namespace s ON s.oid = c.relnamespace
    WHERE s.nspname = 'public' AND pg_get_userbyid(c.relowner) = v_from AND c.relkind IN ('r', 'p', 'v', 'm', 'S');
    IF left_over IS NOT NULL THEN
        RAISE EXCEPTION 'db-owner: still owned by %: %', v_from, left_over;
    END IF;
    SELECT string_agg(p.tablename || '.' || p.policyname, ', ') INTO left_over
    FROM pg_policies p WHERE p.schemaname = 'public' AND v_from = ANY (p.roles);
    IF left_over IS NOT NULL THEN
        RAISE EXCEPTION 'db-owner: policies still for %: %', v_from, left_over;
    END IF;
    -- Every forced table answers to the new owner inside rls.System.
    SELECT string_agg(c.relname, ', ') INTO missing
    FROM pg_class c JOIN pg_namespace s ON s.oid = c.relnamespace
    WHERE s.nspname = 'public' AND c.relforcerowsecurity
      AND NOT EXISTS (SELECT 1 FROM pg_policies p WHERE p.schemaname = 'public' AND p.tablename = c.relname
                      AND p.policyname = 'rls_system' AND v_to = ANY (p.roles));
    IF missing IS NOT NULL THEN
        RAISE EXCEPTION 'db-owner: forced tables without rls_system for %: %', v_to, missing;
    END IF;
    -- The application roles keep their table grants.
    SELECT string_agg(c.relname || ':' || rl, ', ') INTO missing
    FROM pg_class c JOIN pg_namespace s ON s.oid = c.relnamespace
    CROSS JOIN unnest(ARRAY['hakan_rls_runtime', 'hakan_rls_app', 'hakan_rls_worker']) AS rl
    WHERE s.nspname = 'public' AND c.relkind IN ('r', 'p') AND c.relname <> 'schema_migrations'
      AND NOT has_table_privilege(rl, c.oid, 'SELECT');
    IF missing IS NOT NULL THEN
        RAISE EXCEPTION 'db-owner: application roles lost SELECT on: %', missing;
    END IF;
    IF v_to = 'hakan_owner' AND pg_has_role(v_to, 'hakan_rls_runtime', 'USAGE') THEN
        RAISE EXCEPTION 'db-owner: % inherits hakan_rls_runtime (scripts/db-roles.sql grants it WITH INHERIT FALSE)', v_to;
    END IF;
    RAISE NOTICE 'db-owner: verified (owner %, % forced tables with rls_system)', v_to,
        (SELECT count(*) FROM pg_class c JOIN pg_namespace s ON s.oid = c.relnamespace WHERE s.nspname = 'public' AND c.relforcerowsecurity);
END
$$;

\if :dry
\echo 'db-owner: --dry-run: everything above was applied and verified, now rolled back'
ROLLBACK;
\else
COMMIT;
\endif

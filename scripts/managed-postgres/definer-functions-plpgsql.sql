-- Yönetilen Postgres için migration 000089'un alternatifi (issue #317,
-- docs/runbook/db-sahip-gecisi.md "Yönetilen Postgres").
--
-- 000089, politikaların çağırdığı iki SECURITY DEFINER fonksiyona
-- `SET app.system = 'on'` yazar. Süper kullanıcı olmayan sahip bunu ancak
-- `GRANT SET ON PARAMETER app.system` (PostgreSQL 15+, scripts/db-roles.sql)
-- ile yapabilir. Bazı yönetilen sağlayıcılar bu yetkiyi vermez; o zaman
-- 000089 "permission denied to set parameter" ile düşer.
--
-- Bu dosya aynı iki fonksiyonu plpgsql ile yeniden yazar: ayar fonksiyon
-- gövdesinde set_config ile açılır ve dönmeden önce (hata olsa da) eski
-- değerine geri alınır. Çalışma zamanında set_config bir yetki istemez.
-- Davranış 000089 ile aynıdır: politika fonksiyonu sahip olarak, sahibin
-- rls_system politikası altında okur; ayar çağırana taşmaz
-- (TestDefinerFunctionsPlpgsql).
--
-- Kullanım (migration'lar 000089'a kadar uygulandıktan sonra, sahip rolüyle):
--   psql "$OWNER_URL" -v ON_ERROR_STOP=1 -f scripts/managed-postgres/definer-functions-plpgsql.sql
-- Yeni kurulumda: 000088'e kadar migrate up, bu dosya, sonra
-- `migrate force 89` (000089'u uygulanmış say) ve migrate up.
-- Tekrar çalıştırmak güvenlidir.
BEGIN;

CREATE OR REPLACE FUNCTION app_engagement_visible(p_organization_id BIGINT, p_engagement_id BIGINT)
RETURNS BOOLEAN
    LANGUAGE plpgsql VOLATILE SECURITY DEFINER SET search_path = public AS
$$
DECLARE
    prev TEXT := COALESCE(current_setting('app.system', true), '');
    ok   BOOLEAN;
BEGIN
    PERFORM set_config('app.system', 'on', true);
    BEGIN
        ok := p_organization_id = app_current_organization_id()
              AND (app_engagements_read_all()
                   OR EXISTS (SELECT 1 FROM engagement_assignments a
                              WHERE a.organization_id = p_organization_id
                                AND a.engagement_id = p_engagement_id
                                AND a.user_id = app_current_user_id()));
    EXCEPTION WHEN OTHERS THEN
        PERFORM set_config('app.system', prev, true);
        RAISE;
    END;
    PERFORM set_config('app.system', prev, true);
    RETURN ok;
END
$$;

CREATE OR REPLACE FUNCTION engagement_document_visible(p_engagement_id BIGINT, p_doc_date DATE)
RETURNS BOOLEAN
    LANGUAGE plpgsql VOLATILE SECURITY DEFINER SET search_path = public AS
$$
DECLARE
    prev TEXT := COALESCE(current_setting('app.system', true), '');
    ok   BOOLEAN;
BEGIN
    PERFORM set_config('app.system', 'on', true);
    BEGIN
        ok := EXISTS (
            SELECT 1 FROM engagements e
            WHERE e.id = p_engagement_id
              AND (e.ended_at IS NULL OR (e.archive_until IS NOT NULL AND NOW() < e.archive_until))
              AND (e.ended_at IS NULL OR p_doc_date < e.ended_at::date)
              AND (
                  p_doc_date >= e.started_at::date
                  OR NOT EXISTS (
                      SELECT 1 FROM engagements prev_e
                      WHERE prev_e.taxpayer_id = e.taxpayer_id
                        AND prev_e.id <> e.id
                        AND prev_e.organization_id <> e.organization_id
                        AND prev_e.started_at < e.started_at)
                  OR EXISTS (
                      SELECT 1 FROM taxpayer_history_shares s
                      WHERE s.engagement_id = e.id
                        AND s.revoked_at IS NULL
                        AND (s.from_date IS NULL OR p_doc_date >= s.from_date)
                        AND (s.to_date IS NULL OR p_doc_date <= s.to_date))
              ));
    EXCEPTION WHEN OTHERS THEN
        PERFORM set_config('app.system', prev, true);
        RAISE;
    END;
    PERFORM set_config('app.system', prev, true);
    RETURN ok;
END
$$;

-- CREATE OR REPLACE keeps a function's SET clause from 000089 when there was
-- one; drop it, the body does the job now (and the clause needs the grant).
ALTER FUNCTION app_engagement_visible(BIGINT, BIGINT) RESET ALL;
ALTER FUNCTION engagement_document_visible(BIGINT, DATE) RESET ALL;
ALTER FUNCTION app_engagement_visible(BIGINT, BIGINT) SET search_path = public;
ALTER FUNCTION engagement_document_visible(BIGINT, DATE) SET search_path = public;

COMMIT;

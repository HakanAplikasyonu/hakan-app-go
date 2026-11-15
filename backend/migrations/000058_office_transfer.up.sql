-- Ofis geçişi (issue #8, D2): mükellef başka ofise geçince eski ilişki
-- kapanır; geçmiş veri yeni ofise yalnızca mükellefin onayıyla açılır.
--
-- 1. engagements.archive_until: biten engagement'ın salt okunur arşiv
--    süresi. Bitişte ofis ayarından (organizations.settings
--    ->> 'engagement_archive_days', varsayılan 0) hesaplanır.
-- 2. taxpayer_history_shares: mükellefin "geçmiş verimi bu ofisle paylaş"
--    onayı (isteğe bağlı tarih aralığı). Kayıt saklanır; geri alınınca
--    revoked_at dolar.
-- 3. engagement_document_visible(engagement, belge tarihi): bir engagement'ın
--    bir belgeyi görüp göremeyeceği tek yerde tanımlanır.

ALTER TABLE engagements ADD COLUMN archive_until TIMESTAMPTZ NULL;

CREATE TABLE taxpayer_history_shares (
    id              BIGSERIAL   PRIMARY KEY,
    uuid            UUID        NOT NULL DEFAULT gen_random_uuid(),
    taxpayer_id     BIGINT      NOT NULL REFERENCES taxpayers (id) ON DELETE CASCADE,
    organization_id BIGINT      NOT NULL,
    engagement_id   BIGINT      NOT NULL,
    from_date       DATE        NULL,
    to_date         DATE        NULL,
    granted_by      BIGINT      NULL REFERENCES users (id) ON DELETE SET NULL,
    granted_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    revoked_by      BIGINT      NULL REFERENCES users (id) ON DELETE SET NULL,
    revoked_at      TIMESTAMPTZ NULL,
    CONSTRAINT uq_taxpayer_history_shares_uuid UNIQUE (uuid),
    CONSTRAINT chk_taxpayer_history_shares_range CHECK (from_date IS NULL OR to_date IS NULL OR from_date <= to_date),
    CONSTRAINT fk_taxpayer_history_shares_engagement
        FOREIGN KEY (organization_id, engagement_id)
        REFERENCES engagements (organization_id, id) ON DELETE CASCADE
);

CREATE INDEX idx_taxpayer_history_shares_engagement
    ON taxpayer_history_shares (engagement_id)
    WHERE revoked_at IS NULL;
CREATE INDEX idx_taxpayer_history_shares_taxpayer
    ON taxpayer_history_shares (taxpayer_id, granted_at DESC);

SELECT rls_apply_taxpayer('taxpayer_history_shares');

-- Ofisin arşiv gün sayısı (0-3650). Bozuk ayar 0 sayılır.
CREATE OR REPLACE FUNCTION organization_archive_days(p_organization_id BIGINT)
RETURNS INTEGER
    LANGUAGE sql STABLE AS
$$
    SELECT CASE
        WHEN o.settings ->> 'engagement_archive_days' ~ '^[0-9]{1,4}$'
            THEN LEAST((o.settings ->> 'engagement_archive_days')::int, 3650)
        ELSE 0
    END
    FROM organizations o WHERE o.id = p_organization_id;
$$;

-- Bitişte arşiv süresini doldurur (EndEngagement sorguları ended_at'i
-- yazınca, hangi kod yolundan gelirse gelsin).
CREATE OR REPLACE FUNCTION engagements_set_archive_until()
RETURNS trigger AS $$
BEGIN
    IF NEW.ended_at IS NOT NULL AND OLD.ended_at IS NULL AND NEW.archive_until IS NULL THEN
        NEW.archive_until := NEW.ended_at
            + make_interval(days => COALESCE(organization_archive_days(NEW.organization_id), 0));
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_engagements_set_archive_until
    BEFORE UPDATE OF ended_at ON engagements
    FOR EACH ROW
    EXECUTE FUNCTION engagements_set_archive_until();

-- Bir engagement bir belgeyi (belge tarihiyle) görebilir mi:
--   * kendi dönemi: başlangıç günü ve sonrası; bitmişse bitiş gününden öncesi;
--   * bitmiş engagement yalnızca arşiv süresi içinde, yalnızca kendi dönemini;
--   * başlangıçtan önceki (geçmiş) belgeler: mükellefin geri alınmamış paylaşım
--     onayı (tarih aralığı içinde) varsa, ya da mükellefin daha önce başka bir
--     ofisle engagement'ı hiç olmamışsa (ilk ofis: devralınan bir geçmiş yok).
-- SECURITY DEFINER: RLS altında çalışan sorgulardan da aynı kuralı uygular.
CREATE OR REPLACE FUNCTION engagement_document_visible(p_engagement_id BIGINT, p_doc_date DATE)
RETURNS BOOLEAN
    LANGUAGE sql STABLE SECURITY DEFINER SET search_path = public AS
$$
    SELECT EXISTS (
        SELECT 1 FROM engagements e
        WHERE e.id = p_engagement_id
          AND (e.ended_at IS NULL OR (e.archive_until IS NOT NULL AND NOW() < e.archive_until))
          AND (e.ended_at IS NULL OR p_doc_date < e.ended_at::date)
          AND (
              p_doc_date >= e.started_at::date
              OR NOT EXISTS (
                  SELECT 1 FROM engagements prev
                  WHERE prev.taxpayer_id = e.taxpayer_id
                    AND prev.id <> e.id
                    AND prev.organization_id <> e.organization_id
                    AND prev.started_at < e.started_at)
              OR EXISTS (
                  SELECT 1 FROM taxpayer_history_shares s
                  WHERE s.engagement_id = e.id
                    AND s.revoked_at IS NULL
                    AND (s.from_date IS NULL OR p_doc_date >= s.from_date)
                    AND (s.to_date IS NULL OR p_doc_date <= s.to_date))
          )
    );
$$;

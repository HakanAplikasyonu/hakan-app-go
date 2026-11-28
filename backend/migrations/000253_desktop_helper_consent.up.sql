-- Luca yardımcısı: ofis izni ve aktarıcı bilgisayar (luca-ajan-sozlesme.md
-- 1.3). Luca ile ortaklık yok; yardımcının ofis adına Luca'ya girip aktarım
-- yapması için ofisin AÇIK izni gerekir. Kim, ne zaman, hangi metin sürümü ve
-- hangi bilgisayardan onaylandığı saklanır. Luca şifresi sunucuya hiç gelmez
-- (yalnızca o bilgisayarın şifre kasasında durur). İzin yoksa ajan görev
-- üstlenemez (409 CONSENT_REQUIRED).
ALTER TABLE desktop_helper_settings
    ADD COLUMN luca_consent_at           TIMESTAMPTZ NULL,
    ADD COLUMN luca_consent_by           BIGINT      NULL REFERENCES users (id) ON DELETE SET NULL,
    ADD COLUMN luca_consent_agent_id     BIGINT      NULL REFERENCES desktop_agents (id) ON DELETE SET NULL,
    ADD COLUMN luca_consent_text_version TEXT        NULL;
CREATE INDEX idx_desktop_helper_settings_consent_agent ON desktop_helper_settings (luca_consent_agent_id)
    WHERE luca_consent_agent_id IS NOT NULL;

-- Yardımcının son görev yoklaması: panelde "Aktarıcı bilgisayar:
-- çevrimiçi / çevrimdışı, son görülme".
ALTER TABLE desktop_agents ADD COLUMN helper_seen_at TIMESTAMPTZ NULL;

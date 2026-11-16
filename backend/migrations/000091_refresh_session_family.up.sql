-- #323: oturum iptali erişim belirtecini de kessin.
--
-- Her yenileme (rotation) yeni bir refresh_tokens satırı açar; erişim
-- belirtecindeki sid bu yüzden her yenilemede değişiyordu ve çıkış, oturum
-- iptali ya da parola değişimi yalnızca yenileme belirtecini iptal ediyor,
-- verilmiş erişim belirteci süresi dolana kadar (JWT_ACCESS_TTL) geçerli
-- kalıyordu.
--
-- session_uuid: bir girişten doğan bütün satırların ortak, sabit kimliği
-- (erişim belirtecinin sid'i). ended_at: oturum çıkışla, iptalle ya da
-- parola değişimiyle bittiğinde dolar; kimlik yükleyici her istekte bakar.
-- Yenileme yalnızca revoked_at doldurur, oturumu bitirmez.
ALTER TABLE refresh_tokens ADD COLUMN session_uuid UUID;
ALTER TABLE refresh_tokens ADD COLUMN ended_at TIMESTAMPTZ;

UPDATE refresh_tokens SET session_uuid = uuid WHERE session_uuid IS NULL;

-- Doğrudan eklenen bir satır kendi oturumunu başlatır.
ALTER TABLE refresh_tokens ALTER COLUMN session_uuid SET DEFAULT gen_random_uuid();
ALTER TABLE refresh_tokens ALTER COLUMN session_uuid SET NOT NULL;

CREATE INDEX idx_refresh_tokens_session_uuid ON refresh_tokens (session_uuid);

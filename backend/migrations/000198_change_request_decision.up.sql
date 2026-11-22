-- Kilitli döneme gelen değişiklik talebinde seçilen karar (Oğuz'un demo
-- geri bildirimi). Önceden yalnız "uygula" (kayıt kendi dönemine yazılır)
-- ve "reddet" vardı; şimdi müşavir talebi geçmiş döneme düzeltme olarak
-- da işleyebilir ya da seçtiği aya uygulayabilir.
--
-- decision: apply (eski davranış, varsayılan), correction (kilitli
-- döneme düzeltme olarak; düzeltme beyannamesi gerekebilir),
-- target_period (seçilen aya; target_period o ay), reject (reddedildi).
-- Açık talepte boş. Eski kararlar durumdan doldurulur.
--
-- change_requests FORCE altında: veri güncelleyen adım sistem kipinde.
SET LOCAL app.system = 'on';

ALTER TABLE change_requests
    ADD COLUMN decision      TEXT    NULL,
    ADD COLUMN target_period CHAR(7) NULL;

UPDATE change_requests
SET decision = CASE status WHEN 'applied' THEN 'apply' ELSE 'reject' END
WHERE status <> 'open';

ALTER TABLE change_requests
    ADD CONSTRAINT chk_change_requests_decision CHECK (
        (status = 'open' AND decision IS NULL)
        OR (status = 'applied' AND decision IN ('apply', 'correction', 'target_period'))
        OR (status = 'rejected' AND decision = 'reject')),
    ADD CONSTRAINT chk_change_requests_target_period CHECK (
        (decision IS NOT DISTINCT FROM 'target_period') = (target_period IS NOT NULL)
        AND (target_period IS NULL OR target_period ~ '^[0-9]{4}-(0[1-9]|1[0-2])$'));

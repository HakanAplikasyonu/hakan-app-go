-- Kural adayı eşiği ofis ayarı (issue #333, kullanıcı kararı 16 Kasım).
-- Müşavir aynı karşı taraf için aynı hesap koduna rule_candidate_min_count
-- kez karar verince "kural yapılsın mı?" sorulur. Sayılacak kararlar:
-- corrections_only (yalnızca düzeltmeler) ya da corrections_and_accepts
-- (düzeltmeler ve elle onaylar; bugünkü davranış). Otomatik onaylar hiçbir
-- seçenekte sayılmaz. Satırı olmayan ofis varsayılanı kullanır.
CREATE TABLE account_rule_settings (
    organization_id          BIGINT       PRIMARY KEY REFERENCES organizations (id) ON DELETE CASCADE,
    rule_candidate_min_count INTEGER      NOT NULL DEFAULT 3,
    rule_candidate_decisions TEXT         NOT NULL DEFAULT 'corrections_and_accepts',
    updated_by               BIGINT       NULL REFERENCES users (id) ON DELETE SET NULL,
    updated_at               TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    CONSTRAINT chk_account_rule_settings_min CHECK (rule_candidate_min_count BETWEEN 2 AND 10),
    CONSTRAINT chk_account_rule_settings_decisions CHECK (rule_candidate_decisions IN ('corrections_only', 'corrections_and_accepts'))
);

SELECT rls_apply_org('account_rule_settings');

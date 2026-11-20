-- Uyarılı fiş onayı (#669, ofis kararı): eksik belgeli ya da mükerrer
-- olabilecek fiş engellenmez; onaylayan uyarıyı açıkça kabul eder. Kabul
-- edilen uyarılar ve isteğe bağlı gerekçe fişte kalır (kim / ne zaman:
-- approved_by / approved_at).
ALTER TABLE journal_drafts
    ADD COLUMN approval_warnings TEXT[] NOT NULL DEFAULT '{}',
    ADD COLUMN approval_note     TEXT   NULL;

-- Geçmiş veri paylaşım kayıtları ve arşiv süreleri silinir.
DROP FUNCTION IF EXISTS engagement_document_visible(BIGINT, DATE);
DROP TRIGGER IF EXISTS trg_engagements_set_archive_until ON engagements;
DROP FUNCTION IF EXISTS engagements_set_archive_until();
DROP FUNCTION IF EXISTS organization_archive_days(BIGINT);
DROP TABLE IF EXISTS taxpayer_history_shares;
ALTER TABLE engagements DROP COLUMN IF EXISTS archive_until;

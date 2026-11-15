-- Mükellef ekleme talepleri silinir (açılmış engagement'lar kalır).
DELETE FROM notification_templates WHERE code = 'taxpayer.onboarding_consent';
DROP TABLE IF EXISTS taxpayer_onboarding_requests;

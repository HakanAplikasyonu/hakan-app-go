-- Mükellef kullanıcı üyelikleri ve davetleri silinir.
DELETE FROM notification_templates WHERE code = 'taxpayer.invitation';
DROP TABLE IF EXISTS taxpayer_invitations;
DROP TABLE IF EXISTS taxpayer_members;

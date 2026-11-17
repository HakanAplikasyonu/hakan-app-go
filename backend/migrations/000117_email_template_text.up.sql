-- #467: email templates as readable multi-line text. The text part is sent
-- as written; the HTML layout reads its structure: blank line = paragraph,
-- a "label: digits" line = one-time code, a line ending in ":" followed
-- by a lone URL line = button (backend/internal/modules/notifications/providers/email.go).

UPDATE notification_templates SET body = E'Hello,\n\nThe desktop agent {{agent_name}} has been silent for {{minutes}} minutes. Folder watching and local program reads are paused meanwhile.\n\nTo see the agent''s status:\n{{link}}'
    WHERE code = 'agents.offline' AND channel = 'email' AND language = 'en';
UPDATE notification_templates SET body = E'Merhaba,\n\n{{agent_name}} adlı masaüstü ajanından {{minutes}} dakikadır haber alınamıyor. Klasör izleme ve yerel program okuma bu sürede durur.\n\nAjanın durumunu görmek için:\n{{link}}'
    WHERE code = 'agents.offline' AND channel = 'email' AND language = 'tr';
UPDATE notification_templates SET body = E'Hello {{name}},\n\nEnter the code below to confirm deleting (deactivating) your account.\n\nConfirmation code: {{code}}\n\nThe code is valid for {{expires_minutes}} minutes. If you did not request this, we recommend changing your password.'
    WHERE code = 'auth.account_deactivation_code' AND channel = 'email' AND language = 'en';
UPDATE notification_templates SET body = E'Merhaba {{name}},\n\nHesabınızı silme (pasifleştirme) isteğinizi onaylamak için aşağıdaki kodu girin.\n\nOnay kodunuz: {{code}}\n\nKod {{expires_minutes}} dakika geçerlidir. Bu isteği siz yapmadıysanız şifrenizi değiştirmenizi öneririz.'
    WHERE code = 'auth.account_deactivation_code' AND channel = 'email' AND language = 'tr';
UPDATE notification_templates SET body = E'Hello {{name}},\n\nEnter the code below to verify your email address.\n\nVerification code: {{code}}\n\nThe code is valid for {{expires_minutes}} minutes. If you did not request this, you can ignore this email.'
    WHERE code = 'auth.email_verification' AND channel = 'email' AND language = 'en';
UPDATE notification_templates SET body = E'Merhaba {{name}},\n\nE-posta adresinizi doğrulamak için aşağıdaki kodu girin.\n\nDoğrulama kodunuz: {{code}}\n\nKod {{expires_minutes}} dakika geçerlidir. Bu isteği siz yapmadıysanız bu e-postayı yok sayabilirsiniz.'
    WHERE code = 'auth.email_verification' AND channel = 'email' AND language = 'tr';
UPDATE notification_templates SET body = E'Hello {{name}},\n\nYour sign-in code: {{code}}\n\nThe code is valid for {{expires_minutes}} minutes and can be used once.\n\nIf you did not request this, you can ignore this email.'
    WHERE code = 'auth.login_code' AND channel = 'email' AND language = 'en';
UPDATE notification_templates SET body = E'Merhaba {{name}},\n\nGiriş kodunuz: {{code}}\n\nKod {{expires_minutes}} dakika geçerlidir ve yalnızca bir kez kullanılabilir.\n\nBu isteği siz yapmadıysanız bu e-postayı yok sayabilirsiniz.'
    WHERE code = 'auth.login_code' AND channel = 'email' AND language = 'tr';
UPDATE notification_templates SET body = E'Hello {{name}},\n\nEnter the code below to link your {{provider}} sign-in to your existing account.\n\nLinking code: {{code}}\n\nThe code is valid for {{expires_minutes}} minutes. If you did not request this, ignore this email; nothing will be linked.'
    WHERE code = 'auth.oauth_link_code' AND channel = 'email' AND language = 'en';
UPDATE notification_templates SET body = E'Merhaba {{name}},\n\n{{provider}} hesabınızı mevcut hesabınıza bağlamak için aşağıdaki kodu girin.\n\nBağlama kodunuz: {{code}}\n\nKod {{expires_minutes}} dakika geçerlidir. Bu isteği siz yapmadıysanız bu e-postayı yok sayın; hesabınıza bir şey bağlanmaz.'
    WHERE code = 'auth.oauth_link_code' AND channel = 'email' AND language = 'tr';
UPDATE notification_templates SET body = E'Hello {{name}},\n\nEnter the code below to reset your password.\n\nPassword reset code: {{code}}\n\nThe code is valid for {{expires_minutes}} minutes. If you did not request this, you can ignore this email; your password stays the same.'
    WHERE code = 'auth.password_reset' AND channel = 'email' AND language = 'en';
UPDATE notification_templates SET body = E'Merhaba {{name}},\n\nŞifrenizi sıfırlamak için aşağıdaki kodu girin.\n\nŞifre sıfırlama kodunuz: {{code}}\n\nKod {{expires_minutes}} dakika geçerlidir. Bu isteği siz yapmadıysanız bu e-postayı yok sayabilirsiniz; şifreniz değişmez.'
    WHERE code = 'auth.password_reset' AND channel = 'email' AND language = 'tr';
UPDATE notification_templates SET subject = E'Your account is ready', body = E'Hello {{name}},\n\nYour account was created and is ready to use.'
    WHERE code = 'auth.welcome' AND channel = 'email' AND language = 'en';
UPDATE notification_templates SET subject = E'Hesabınız hazır', body = E'Merhaba {{name}},\n\nHesabınız oluşturuldu ve kullanıma hazır.'
    WHERE code = 'auth.welcome' AND channel = 'email' AND language = 'tr';
UPDATE notification_templates SET body = E'Hello,\n\n{{org_name}} requests documents (invoice, receipt) for {{count}} bank transactions of {{taxpayer_name}}. You can upload a photo or PDF in the app.\n\nTo upload the documents:\n{{link}}'
    WHERE code = 'doc_tasks.created' AND channel = 'email' AND language = 'en';
UPDATE notification_templates SET body = E'Merhaba,\n\n{{org_name}}, {{taxpayer_name}} için {{count}} banka hareketinin belgesini (fatura, fiş, dekont) istiyor. Fotoğrafını ya da PDF''ini uygulamadan yükleyebilirsiniz.\n\nBelgeleri yüklemek için:\n{{link}}'
    WHERE code = 'doc_tasks.created' AND channel = 'email' AND language = 'tr';
UPDATE notification_templates SET body = E'Hello,\n\n{{org_name}} is still waiting for documents for {{count}} bank transactions of {{taxpayer_name}}.\n\nTo upload the documents:\n{{link}}'
    WHERE code = 'doc_tasks.reminder' AND channel = 'email' AND language = 'en';
UPDATE notification_templates SET body = E'Merhaba,\n\n{{org_name}}, {{taxpayer_name}} için {{count}} banka hareketinin belgesini hâlâ bekliyor.\n\nBelgeleri yüklemek için:\n{{link}}'
    WHERE code = 'doc_tasks.reminder' AND channel = 'email' AND language = 'tr';
UPDATE notification_templates SET body = E'Hello,\n\n{{org_name}} requests a document for {{taxpayer_name}}: {{title}}\n\nYou can upload a photo or PDF in the app:\n{{link}}'
    WHERE code = 'doc_tasks.requested' AND channel = 'email' AND language = 'en';
UPDATE notification_templates SET body = E'Merhaba,\n\n{{org_name}}, {{taxpayer_name}} için belge istiyor: {{title}}\n\nFotoğrafını ya da PDF''ini uygulamadan yükleyebilirsiniz:\n{{link}}'
    WHERE code = 'doc_tasks.requested' AND channel = 'email' AND language = 'tr';
UPDATE notification_templates SET body = E'Hello,\n\n{{taxpayer_name}} uploaded the requested document: {{title}}\n\nTo review the document:\n{{link}}'
    WHERE code = 'doc_tasks.submitted' AND channel = 'email' AND language = 'en';
UPDATE notification_templates SET body = E'Merhaba,\n\n{{taxpayer_name}} istenen belgeyi yükledi: {{title}}\n\nBelgeyi incelemek için:\n{{link}}'
    WHERE code = 'doc_tasks.submitted' AND channel = 'email' AND language = 'tr';
UPDATE notification_templates SET body = E'Hello,\n\nThis is a test email. If you can read it, your email notifications work.'
    WHERE code = 'notifications.test' AND channel = 'email' AND language = 'en';
UPDATE notification_templates SET body = E'Merhaba,\n\nBu bir test e-postasıdır. Bu e-postayı görüyorsanız e-posta bildirimleriniz çalışıyor.'
    WHERE code = 'notifications.test' AND channel = 'email' AND language = 'tr';
UPDATE notification_templates SET body = E'Hello,\n\n{{inviter_name}} ({{org_name}}) invited you to the taxpayer panel of {{taxpayer_name}} as {{role_name}}.\n\nTo accept the invitation:\n{{link}}\n\nThe link is valid for {{expires_days}} days. If you did not expect this invitation, ignore this email.'
    WHERE code = 'taxpayer.invitation' AND channel = 'email' AND language = 'en';
UPDATE notification_templates SET body = E'Merhaba,\n\n{{inviter_name}} ({{org_name}}) sizi {{taxpayer_name}} şirketinin mükellef paneline {{role_name}} olarak davet etti.\n\nDaveti kabul etmek için:\n{{link}}\n\nBağlantı {{expires_days}} gün geçerlidir. Bu daveti beklemiyorsanız e-postayı yok sayabilirsiniz.'
    WHERE code = 'taxpayer.invitation' AND channel = 'email' AND language = 'tr';
UPDATE notification_templates SET body = E'Hello,\n\nThe accounting office {{org_name}} wants to open a client record for {{title}} ({{tax_id}}).\n\nTo approve or decline:\n{{link}}\n\nThe link is valid for {{expires_days}} days. If you did not expect this, ignore this email.'
    WHERE code = 'taxpayer.onboarding_consent' AND channel = 'email' AND language = 'en';
UPDATE notification_templates SET body = E'Merhaba,\n\n{{org_name}} mali müşavirlik ofisi {{title}} ({{tax_id}}) için mükellef kaydı açmak istiyor.\n\nOnaylamak ya da reddetmek için:\n{{link}}\n\nBağlantı {{expires_days}} gün geçerlidir. Bu talebi beklemiyorsanız e-postayı yok sayabilirsiniz.'
    WHERE code = 'taxpayer.onboarding_consent' AND channel = 'email' AND language = 'tr';
UPDATE notification_templates SET body = E'Hello,\n\n{{inviter_name}} invited you to join {{org_name}} as {{role_name}}.\n\nTo accept the invitation:\n{{link}}\n\nThe link is valid for {{expires_days}} days. If you did not expect this invitation, ignore this email.'
    WHERE code = 'tenant.invitation' AND channel = 'email' AND language = 'en';
UPDATE notification_templates SET body = E'Merhaba,\n\n{{inviter_name}} sizi {{org_name}} organizasyonuna {{role_name}} olarak davet etti.\n\nDaveti kabul etmek için:\n{{link}}\n\nBağlantı {{expires_days}} gün geçerlidir. Bu daveti beklemiyorsanız e-postayı yok sayabilirsiniz.'
    WHERE code = 'tenant.invitation' AND channel = 'email' AND language = 'tr';
UPDATE notification_templates SET body = E'Hello,\n\n{{count}} blocking warnings were raised for {{taxpayer_name}}. The period is not ready until they are closed.\n\nTo see the warnings:\n{{link}}'
    WHERE code = 'warnings.blocker' AND channel = 'email' AND language = 'en';
UPDATE notification_templates SET body = E'Merhaba,\n\n{{taxpayer_name}} için {{count}} engelleyici uyarı açıldı. Engelleyiciler kapanmadan dönem beyana hazır sayılmaz.\n\nUyarıları görmek için:\n{{link}}'
    WHERE code = 'warnings.blocker' AND channel = 'email' AND language = 'tr';

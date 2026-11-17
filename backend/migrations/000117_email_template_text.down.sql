-- Restores the single-line email texts of the migrations that added them.

UPDATE notification_templates SET body = E'Hello, the desktop agent {{agent_name}} has been silent for {{minutes}} minutes. Folder watching and local program reads are paused meanwhile. Details: {{link}}'
    WHERE code = 'agents.offline' AND channel = 'email' AND language = 'en';
UPDATE notification_templates SET body = E'Merhaba, {{agent_name}} adlı masaüstü ajanından {{minutes}} dakikadır haber alınamıyor. Klasör izleme ve yerel program okuma bu sürede durur. Ayrıntılar: {{link}}'
    WHERE code = 'agents.offline' AND channel = 'email' AND language = 'tr';
UPDATE notification_templates SET body = E'Hello {{name}},\n\nYour code to confirm deleting (deactivating) your account is: {{code}}\nThe code is valid for {{expires_minutes}} minutes.\n\nIf you did not request this, we recommend changing your password.'
    WHERE code = 'auth.account_deactivation_code' AND channel = 'email' AND language = 'en';
UPDATE notification_templates SET body = E'Merhaba {{name}},\n\nHesabınızı silme (pasifleştirme) isteğinizi onaylamak için kodunuz: {{code}}\nKod {{expires_minutes}} dakika geçerlidir.\n\nBu isteği siz yapmadıysanız şifrenizi değiştirmenizi öneririz.'
    WHERE code = 'auth.account_deactivation_code' AND channel = 'email' AND language = 'tr';
UPDATE notification_templates SET body = E'Hello {{name}}, your verification code is: {{code}}. Valid for {{expires_minutes}} minutes.'
    WHERE code = 'auth.email_verification' AND channel = 'email' AND language = 'en';
UPDATE notification_templates SET body = E'Merhaba {{name}}, doğrulama kodunuz: {{code}}. Geçerlilik: {{expires_minutes}} dakika.'
    WHERE code = 'auth.email_verification' AND channel = 'email' AND language = 'tr';
UPDATE notification_templates SET body = E'Hello {{name}},\n\nYour sign-in code is: {{code}}\nThe code is valid for {{expires_minutes}} minutes and can be used once.\n\nIf you did not request this, you can ignore this email.'
    WHERE code = 'auth.login_code' AND channel = 'email' AND language = 'en';
UPDATE notification_templates SET body = E'Merhaba {{name}},\n\nGiriş kodunuz: {{code}}\nKod {{expires_minutes}} dakika geçerlidir ve yalnızca bir kez kullanılabilir.\n\nBu isteği siz yapmadıysanız bu e-postayı yok sayabilirsiniz.'
    WHERE code = 'auth.login_code' AND channel = 'email' AND language = 'tr';
UPDATE notification_templates SET body = E'Hello {{name}},\n\nYour code to link your {{provider}} sign-in to your existing account is: {{code}}\nThe code is valid for {{expires_minutes}} minutes.\n\nIf you did not request this, ignore this email; nothing will be linked.'
    WHERE code = 'auth.oauth_link_code' AND channel = 'email' AND language = 'en';
UPDATE notification_templates SET body = E'Merhaba {{name}},\n\n{{provider}} hesabınızı mevcut hesabınıza bağlamak için kodunuz: {{code}}\nKod {{expires_minutes}} dakika geçerlidir.\n\nBu isteği siz yapmadıysanız bu e-postayı yok sayın; hesabınıza bir şey bağlanmaz.'
    WHERE code = 'auth.oauth_link_code' AND channel = 'email' AND language = 'tr';
UPDATE notification_templates SET body = E'Hello {{name}}, your password reset code is: {{code}}. Valid for {{expires_minutes}} minutes.'
    WHERE code = 'auth.password_reset' AND channel = 'email' AND language = 'en';
UPDATE notification_templates SET body = E'Merhaba {{name}}, şifre sıfırlama kodunuz: {{code}}. Geçerlilik: {{expires_minutes}} dakika.'
    WHERE code = 'auth.password_reset' AND channel = 'email' AND language = 'tr';
UPDATE notification_templates SET subject = E'Welcome to App', body = E'Hello {{name}}, your App account was created successfully.'
    WHERE code = 'auth.welcome' AND channel = 'email' AND language = 'en';
UPDATE notification_templates SET subject = E'App''e hoş geldiniz', body = E'Merhaba {{name}}, App hesabınız başarıyla oluşturuldu.'
    WHERE code = 'auth.welcome' AND channel = 'email' AND language = 'tr';
UPDATE notification_templates SET body = E'Hello, {{org_name}} requests documents (invoice, receipt) for {{count}} bank transactions of {{taxpayer_name}}. Upload a photo or PDF in the app: {{link}}'
    WHERE code = 'doc_tasks.created' AND channel = 'email' AND language = 'en';
UPDATE notification_templates SET body = E'Merhaba, {{org_name}} {{taxpayer_name}} için {{count}} banka hareketinin belgesini (fatura, fiş, dekont) istiyor. Uygulamadan fotoğrafını ya da PDF''ini yükleyebilirsiniz: {{link}}'
    WHERE code = 'doc_tasks.created' AND channel = 'email' AND language = 'tr';
UPDATE notification_templates SET body = E'Hello, {{org_name}} is still waiting for documents for {{count}} bank transactions of {{taxpayer_name}}. Upload them here: {{link}}'
    WHERE code = 'doc_tasks.reminder' AND channel = 'email' AND language = 'en';
UPDATE notification_templates SET body = E'Merhaba, {{org_name}} {{taxpayer_name}} için {{count}} banka hareketinin belgesini hâlâ bekliyor. Yüklemek için: {{link}}'
    WHERE code = 'doc_tasks.reminder' AND channel = 'email' AND language = 'tr';
UPDATE notification_templates SET body = E'Hello, {{org_name}} requests a document for {{taxpayer_name}}: {{title}}. Upload a photo or PDF in the app: {{link}}'
    WHERE code = 'doc_tasks.requested' AND channel = 'email' AND language = 'en';
UPDATE notification_templates SET body = E'Merhaba, {{org_name}} {{taxpayer_name}} için belge istiyor: {{title}}. Uygulamadan fotoğrafını ya da PDF''ini yükleyebilirsiniz: {{link}}'
    WHERE code = 'doc_tasks.requested' AND channel = 'email' AND language = 'tr';
UPDATE notification_templates SET body = E'Hello, {{taxpayer_name}} uploaded the requested document: {{title}}. Review it here: {{link}}'
    WHERE code = 'doc_tasks.submitted' AND channel = 'email' AND language = 'en';
UPDATE notification_templates SET body = E'Merhaba, {{taxpayer_name}} istenen belgeyi yükledi: {{title}}. İncelemek için: {{link}}'
    WHERE code = 'doc_tasks.submitted' AND channel = 'email' AND language = 'tr';
UPDATE notification_templates SET body = E'This is a test email.'
    WHERE code = 'notifications.test' AND channel = 'email' AND language = 'en';
UPDATE notification_templates SET body = E'Bu bir test e-postasıdır.'
    WHERE code = 'notifications.test' AND channel = 'email' AND language = 'tr';
UPDATE notification_templates SET body = E'Hello, {{inviter_name}} ({{org_name}}) invited you to the taxpayer panel of {{taxpayer_name}} as {{role_name}}. Open this link to accept: {{link}} (The link is valid for {{expires_days}} days.)'
    WHERE code = 'taxpayer.invitation' AND channel = 'email' AND language = 'en';
UPDATE notification_templates SET body = E'Merhaba, {{inviter_name}} ({{org_name}}) sizi {{taxpayer_name}} şirketinin mükellef paneline {{role_name}} olarak davet etti. Daveti kabul etmek için bağlantıyı açın: {{link}} (Bağlantı {{expires_days}} gün geçerlidir.)'
    WHERE code = 'taxpayer.invitation' AND channel = 'email' AND language = 'tr';
UPDATE notification_templates SET body = E'Hello, the accounting office {{org_name}} wants to open a client record for {{title}} ({{tax_id}}). Open this link to approve or decline: {{link}} (The link is valid for {{expires_days}} days.) If you did not expect this, ignore this email.'
    WHERE code = 'taxpayer.onboarding_consent' AND channel = 'email' AND language = 'en';
UPDATE notification_templates SET body = E'Merhaba, {{org_name}} mali müşavirlik ofisi {{title}} ({{tax_id}}) için mükellef kaydı açmak istiyor. Onaylamak ya da reddetmek için bağlantıyı açın: {{link}} (Bağlantı {{expires_days}} gün geçerlidir.) Bu talebi siz beklemiyorsanız e-postayı yok sayabilirsiniz.'
    WHERE code = 'taxpayer.onboarding_consent' AND channel = 'email' AND language = 'tr';
UPDATE notification_templates SET body = E'Hello, {{inviter_name}} invited you to join {{org_name}} as {{role_name}}. Open this link to accept: {{link}} (The link is valid for {{expires_days}} days.)'
    WHERE code = 'tenant.invitation' AND channel = 'email' AND language = 'en';
UPDATE notification_templates SET body = E'Merhaba, {{inviter_name}} sizi {{org_name}} organizasyonuna {{role_name}} olarak davet etti. Daveti kabul etmek için bağlantıyı açın: {{link}} (Bağlantı {{expires_days}} gün geçerlidir.)'
    WHERE code = 'tenant.invitation' AND channel = 'email' AND language = 'tr';
UPDATE notification_templates SET body = E'Hello, {{count}} blocking warnings were raised for {{taxpayer_name}}. The period is not ready until they are closed: {{link}}'
    WHERE code = 'warnings.blocker' AND channel = 'email' AND language = 'en';
UPDATE notification_templates SET body = E'Merhaba, {{taxpayer_name}} için {{count}} engelleyici uyarı açıldı. Engelleyiciler kapanmadan dönem beyana hazır sayılmaz: {{link}}'
    WHERE code = 'warnings.blocker' AND channel = 'email' AND language = 'tr';

# E-posta teslimi: DNS ve gönderen kontrol listesi (#467)

Uygulamanın gönderdiği e-postaların (giriş kodları, davetler, bildirimler,
mutabakat mektupları, mükellef raporları) gelen kutusuna düşmesi için
canlıya çıkmadan önce bu listeyi tamamlayın. Örneklerde alan adı
`ornek.com.tr`, uygulama adresi `https://app.ornek.com.tr`'dir. Kendi
değerlerinizle değiştirin.

E-postaların kendisi zaten teslim kurallarına uyar: her e-posta
`multipart/alternative` (okunur düz metin + HTML), HTML 102 KB'ın altında,
`Message-ID`, `Date`, UTF-8 konu, bildirimlerde tek tık abonelikten çıkma.
Bunları altın dosya testleri denetler (`backend/internal/platform/mail`,
`backend/internal/platform/mailtpl`). Bu belge altyapı tarafını kapsar.

## 1. Gönderen kimliği (`.env.server`)

| Değişken | Değer | Not |
|---|---|---|
| `MAIL_FROM` | `noreply@ornek.com.tr` | Zarf göndereni ve `From`. Alan adı aşağıdaki SPF/DKIM/DMARC kayıtlarının alan adıdır. `Message-ID` da bu alan adında üretilir. |
| `MAIL_FROM_NAME` | Ürünün görünen adı | Boş bırakılırsa `compose.prod.yml` varsayılanı `App` görünür; mutlaka doldurun. |
| `MAIL_REPLY_TO` | `destek@ornek.com.tr` | İsteğe bağlı. Boşsa `Reply-To` başlığı eklenmez, yanıtlar `MAIL_FROM`'a gider. Okunan bir kutu olmalı. |
| `SMTP_HOST`, `SMTP_PORT` | SMTP sağlayıcısı | 587 (STARTTLS) ya da 465 (doğrudan TLS). |
| `SMTP_TLS` | `starttls` ya da `implicit` | Üretimde `auto` ya da `none` kullanmayın. |
| `PUBLIC_FRONTEND_URL` | `https://app.ornek.com.tr` | **https olmalı.** Düğmeler, logo ve abonelikten çıkma bağlantısı bu adresten üretilir. http ise e-posta yine gider ama düğmeler bağlantısız metne iner. |
| `APP_ENCRYPTION_KEY` | (üretilmiş) | Abonelikten çıkma bağlantılarının imza anahtarı bundan türetilir. Asla döndürmeyin (`@norotate`). Değişirse eski e-postalardaki bağlantılar geçersiz olur. |

- [ ] `MAIL_FROM` alan adı uygulamanın kendi alan adı (ya da alt alan adı). Ücretsiz posta adresi (gmail.com vb.) kullanılmaz; DMARC bunları reddeder.
- [ ] `MAIL_FROM` adresine gelen geri dönüşler (bounce) bir yerde toplanıyor ya da sağlayıcının geri dönüş adresi (Return-Path) aynı alan adının alt alanında.

## 2. SPF

Gönderen alan adında **tek** bir SPF kaydı olur ve SMTP sağlayıcısını kapsar.

```
ornek.com.tr.  TXT  "v=spf1 include:<sağlayıcının-spf-alanı> -all"
```

- [ ] Yalnızca bir `v=spf1` kaydı var (iki kayıt SPF'yi geçersiz kılar).
- [ ] Toplam DNS sorgusu 10'u aşmıyor (`include`, `a`, `mx`, `redirect` sayılır).
- [ ] Sağlayıcı ayrı bir geri dönüş alan adı (ör. `bounce.ornek.com.tr`) kullanıyorsa onun SPF/CNAME kaydı da sağlayıcının belgesine göre eklendi. DMARC hizalaması için bu alan `ornek.com.tr`'nin alt alanı olmalı.
- [ ] Kontrol: `dig +short TXT ornek.com.tr | grep spf1`

## 3. DKIM

Sağlayıcının verdiği seçici (selector) ve açık anahtar DNS'e eklenir. İmza
`d=ornek.com.tr` ile atılmalı (From alan adıyla hizalı).

```
s1._domainkey.ornek.com.tr.  TXT    "v=DKIM1; k=rsa; p=MIIBIjANBgkq…"
# ya da sağlayıcı CNAME veriyorsa:
s1._domainkey.ornek.com.tr.  CNAME  s1.<sağlayıcının-dkim-alanı>.
```

- [ ] Anahtar en az 2048 bit.
- [ ] Sağlayıcı panelinde alan adı "doğrulandı" görünüyor.
- [ ] Kontrol: `dig +short TXT s1._domainkey.ornek.com.tr`

## 4. DMARC

SPF ve DKIM çalıştıktan sonra eklenir. Önce yalnız izleme, raporlar temiz
gelince sıkılaştırma.

```
_dmarc.ornek.com.tr.  TXT  "v=DMARC1; p=none; rua=mailto:dmarc@ornek.com.tr; adkim=r; aspf=r"
```

- [ ] `rua` adresi okunan bir kutu (günlük toplu raporlar gelir).
- [ ] 2–4 hafta raporlar temizse `p=quarantine`, ardından `p=reject`.
- [ ] Kontrol: `dig +short TXT _dmarc.ornek.com.tr`

Gmail ve Yahoo günde 5.000'den fazla e-posta gönderenlerden SPF, DKIM ve
DMARC'ın üçünü, tek tık abonelikten çıkmayı (bkz. 7) ve %0,3'ün altında
spam şikâyeti oranını ister. Küçük hacimde de aynısını uygulayın.

## 5. Ters DNS (PTR) ve HELO

Bir SMTP sağlayıcısının sunucusu üzerinden gönderiyorsanız (önerilen) bu
adım sağlayıcıdadır; yalnızca 5.1'i kontrol edin.

- [ ] 5.1 Sağlayıcının gönderen IP'leri için PTR kaydı var (sağlayıcı belgesi).

Kendi sunucunuzdan (kendi IP'nizden) doğrudan gönderiyorsanız:

- [ ] Sunucu IP'sinin PTR kaydı bir ana makine adını gösteriyor (ör. `posta.ornek.com.tr`); bu kaydı IP'nin sahibi (barındırma firması) ekler.
- [ ] O ana makine adının A/AAAA kaydı aynı IP'yi gösteriyor (ileri-geri eşleşme).
- [ ] SMTP sunucusu `EHLO`'da aynı ana makine adını söylüyor.
- [ ] IP hiçbir kara listede değil.
- [ ] Kontrol: `dig +short -x <IP>` ve `dig +short posta.ornek.com.tr`

## 6. Aktarım güvenliği

- [ ] `SMTP_TLS=starttls` (587) ya da `implicit` (465). `auto` üretimde STARTTLS reklamı silinirse düz metne düşebilir.
- [ ] İsteğe bağlı: gelen posta için MTA-STS ve TLS-RPT (`_mta-sts`, `_smtp._tls` kayıtları). Gönderimi etkilemez, alan adının itibarına katkıdır.

## 7. Abonelikten çıkma ve görseller

Bildirim e-postaları (belge talepleri, uyarılar, masaüstü ajanı, hoş
geldiniz) `List-Unsubscribe` ve `List-Unsubscribe-Post` taşır. Güvenlik ve
işlem e-postaları (kodlar, davetler, mutabakat, rapor) taşımaz.

- [ ] `https://app.ornek.com.tr/api/v1/notifications/unsubscribe` dışarıdan HTTPS ile erişilebilir. Posta sağlayıcısı buraya oturumsuz `POST` atar; ters vekil (Caddy/Dokploy) bu yolu engellememeli.
- [ ] Aynı adrese tarayıcıyla `GET`: değişiklik yapmadan `/profile` (bildirim tercihleri) sayfasına yönlendirir.
- [ ] Logo oturumsuz açılıyor: `https://app.ornek.com.tr/email/logo-96.png` (200, `image/png`).

## 8. Canlı öncesi deneme

1. [ ] `make mail-preview` ile bütün e-postaları yerelde açın (`backend/tmp/mail-preview/index.html`); görünüm ve metinler doğru.
2. [ ] Üretimde bir test kullanıcısıyla test e-postası gönderin (API: `POST /v1/notifications/test`, gövde `{"email": true}`) ve bir giriş kodu isteyin. Alıcılar: en az bir Gmail ve bir Outlook test kutusu.
3. [ ] Gelen e-postada "Orijinali göster" / "Mesaj kaynağı": `SPF: PASS`, `DKIM: PASS`, `DMARC: PASS`; `From` alan adı ile DKIM `d=` aynı.
4. [ ] E-posta gelen kutusunda (spam değil); ön izleme satırı (preheader) anlamlı; karanlık modda okunur; görseller kapalıyken metin okunur.
5. [ ] Bildirim türü bir e-postada istemcinin "Aboneliği iptal et" bağlantısı görünüyor; test kullanıcısında tıklayınca bildirim tercihlerinde e-posta kapanıyor.

## Sorun giderme

| Belirti | Olası neden |
|---|---|
| Hepsi spam'e düşüyor | DKIM yok ya da `d=` alan adı `From` ile hizalı değil; DMARC `p=reject` iken SPF/DKIM başarısız. |
| "via sağlayıcı.com" görünüyor | DKIM sağlayıcının kendi alan adıyla atılıyor; alan adı doğrulaması yapılmamış. |
| Bazı alıcılarda geri dönüyor (550) | SPF'de sağlayıcı yok, PTR eksik ya da IP kara listede. |
| Düğmeler tıklanmıyor, adres düz metin | `PUBLIC_FRONTEND_URL` https değil. |
| Logo görünmüyor | Logo adresi oturum istiyor ya da https değil; istemci görselleri kapatmış (metin yine okunur). |
| Gmail "[İleti kırpıldı]" diyor | HTML 102 KB'ı aştı; testler bunu yakalamalıydı, `make mail-preview` çıktısındaki boyut sütununa bakın. |
| Abonelikten çıkma çalışmıyor | `APP_ENCRYPTION_KEY` değişti (eski bağlantılar geçersiz) ya da `/api/v1/notifications/unsubscribe` vekilde engelli. |

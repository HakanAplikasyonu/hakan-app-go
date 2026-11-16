# Hız sınırlama ve kötüye kullanım koruması (#180)

Giriş, kayıt, şifre sıfırlama, davet ve dosya yükleme uçları kötüye kullanıma
karşı üç katmanla korunur: sabit pencereli hız sınırı, başarısız girişte artan
bekleme ve yüklenen dosyanın içerik denetimi. Engellenen her istek denetim
kaydına (`activity_events`) düşer.

## Depo

`RATE_LIMIT_STORE` sayaçların nerede tutulacağını seçer:

| Değer    | Davranış |
|----------|----------|
| `auto`   | Varsayılan. Redis yapılandırılmışsa Redis, değilse süreç içi bellek. |
| `redis`  | Yalnızca Redis. Redis yoksa hiçbir şey sınırlanmaz. |
| `memory` | Süreç içi bellek. Birden çok API kopyası varsa her kopya kendi payını sınırlar. |
| `off`    | Sınır yok (yalnızca geliştirme ve yük testi için). |

Depo hatası (Redis erişilemiyor, bellek deposu dolu) isteği **geçirir**: koruma
kesintisi hizmet kesintisine dönüşmez. Bellek deposu en çok 100 000 anahtar tutar,
süresi dolanları dakikada bir temizler.

## Sabit pencereler

| Eylem         | Anahtar                    | Varsayılan  | Ortam değişkeni |
|---------------|----------------------------|-------------|-----------------|
| `login`       | IP + e-posta               | 10 / 15 dk  | `RATE_LIMIT_LOGIN_LIMIT`, `_WINDOW` |
| `register`    | IP                         | 5 / 15 dk   | `RATE_LIMIT_REGISTER_*` |
| `forgot`      | IP + e-posta               | 5 / 15 dk   | `RATE_LIMIT_FORGOT_*` |
| `reset`       | IP                         | 10 / 15 dk  | `RATE_LIMIT_RESET_*` |
| `upload_user` | oturum belirtecinin özeti  | 300 / 10 dk | `RATE_LIMIT_UPLOAD_USER_*` |
| `upload_ip`   | IP                         | 600 / 10 dk | `RATE_LIMIT_UPLOAD_IP_*` |

Davet uçlarının (ofis daveti, mükellef paneli daveti, davet önizleme ve kabul,
mükellef onayı) sınırları kendi modüllerinde tanımlıdır
(`internal/modules/tenant`, `internal/modules/taxpayers`). Aynı depoyu
paylaşırlar ve engellendiklerinde aynı denetim olayını yazarlar.

Yükleme sınırı tüm API'nin önünde bir ara katmandır (`ratelimit.UploadGuard`):
`multipart/*` gövdeli her yazma isteği ile yolunda `upload(s)`, `ubl-uploads`,
`bank-statements`, `import(s)`, `excel-imports` veya `logo` bölümü geçen yazma
istekleri sayılır. Gövde okunmadan önce karar verilir. Belirtecin kendisi hiçbir
yerde saklanmaz, yalnızca SHA-256 özetinin ilk 16 karakteri anahtar olur.

Engellenen istek `429 Too Many Requests` ve `Retry-After` başlığıyla döner.

## Başarısız girişte artan bekleme

Yanlış şifre veya yanlış doğrulama kodu hem hesap (e-posta) hem IP için sayılır
(`LOGIN_BACKOFF_WINDOW`, varsayılan 1 saat). Eşik aşıldıktan sonraki her
başarısızlık anahtarı kilitler:

```
bekleme = LOGIN_BACKOFF_BASE × 2^(başarısızlık − eşik), en çok LOGIN_BACKOFF_MAX
```

Varsayılanlarla hesap için 5. başarısızlıkta 30 sn, sonra 1, 2, 4, 8 ve 15 dk.
IP eşiği 20'dir (aynı ofisten, NAT arkasından giren kullanıcılar birbirini
kilitlemesin diye). Kilit sürerken doğru şifre de reddedilir. Yanıt, pencere
sınırıyla aynı 429'dur, dolayısıyla hesabın var olup olmadığını ele vermez.
Başarılı giriş hesabın sayacını sıfırlar. IP sayacı sıfırlanmaz, çünkü tek bir
geçerli hesap bir parola püskürtme saldırısını temizleyememeli.

## Yükleme içerik denetimi

Dosya yöneticisinin genel yükleme ucu (`POST /v1/platform/storage/uploads`)
dosyanın ilk 512 baytına bakar (`internal/platform/filetype`):

- Çalıştırılabilir dosyalar (PE/`MZ`, ELF, Mach-O), `#!` ile başlayan betikler,
  HTML içeriği ve `.exe`, `.bat`, `.ps1`, `.js`, `.html`… uzantıları reddedilir.
- Beyan edilen tür `image/*` veya `application/pdf` ise içerik bunu doğrulamalı,
  yoksa reddedilir.
- Saklanan tür istemcinin beyanı değil, doğrulanmış türdür.

Ret yanıtı `415 UNSUPPORTED_FILE_TYPE`, denetim olayı `security.upload_rejected`
olur. Alana özel yükleme uçları zaten kendi denetimini yapar: e-fatura (XML/ZIP),
belge görevleri (PDF/JPEG/PNG/WebP), logo (görsel türleri), banka ekstresi ve
Excel aktarımları (ayrıştırıcı). Boyut sınırları her uçta `http.MaxBytesReader`
ile gövde okunmadan önce uygulanır.

## Denetim kaydı

| Olay                      | Ne zaman | İçerik |
|---------------------------|----------|--------|
| `security.rate_limited`   | Pencere sınırı aşıldı | eylem, anahtar özeti, bekleme, yöntem, yol |
| `security.login_locked`   | Artan bekleme kilidi başladı veya kilitliyken deneme yapıldı | maskeli e-posta (`f***@ornek.com`), anahtar özeti, bekleme |
| `security.upload_rejected`| Yükleme içerik denetiminden geçmedi | neden (`blocked_type` / `type_mismatch`), uzantı |

IP ve tarayıcı bilgisi olayın kendi sütunlarına yazılır. Bir saldırı denetim
tablosunu doldurmasın diye aynı (olay, eylem, anahtar) için pencere veya kilit
süresince (en az 1 dakika) tek kayıt düşülür.

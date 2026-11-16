# Alan düzeyinde şifreleme (#168)

Kaynaktan gelen ham veri (banka hareketinin ham satırı, faturanın ve carinin
ham kaydı, fiş OCR özeti, entegratör liste başlığı) JSONB kolonlarda düz metin
durmaz. Her değer [sır kasasıyla](sir-kasasi.md) aynı zarf şifrelemeyle
mühürlenir, ama **ayrı bir anahtar halkasıyla**: `APP_ENCRYPTION_KEY` ya da
`SECRETS_KEYRING` sızsa bile bu kolonlar açılmaz.

Kod: `internal/platform/secrets/field.go` (şifre), `internal/platform/fieldcrypt`
(yapılandırma ve veri geçişi), `cmd/field-crypt` (komut).

## Kolonlar

| Kolon | Amaç (AAD) |
|---|---|
| `bank_transactions.raw_payload`, `bank_transaction_sources.raw_payload` | `bank.raw_payload` |
| `invoices.raw_payload`, `invoice_sources.raw_payload` | `invoice.raw_payload` |
| `customers.raw_payload`, `customer_source_links.raw_payload` | `customer.raw_payload` |
| `expenses.raw_payload` | `expense.raw_payload` |
| `retail_sales.raw_payload` | `retail_sale.raw_payload` |
| `entegrator_documents.header_raw` | `entegrator.header_raw` |

Aralarında değer kopyalanan kolonlar (banka kaynağı → hareket) aynı amacı
paylaşır. Amaç AAD'ye girer: bir değer başka türde bir kolona taşınırsa açılmaz.
Değer satır kimliğine bağlı değildir (satır yazılmadan önce şifrelenir).

## Biçim

Kolon tipi JSONB kalır; mühürlü değer geçerli bir JSON nesnesidir:

```
{"$enc":"f1","kek":"<KEK kimliği>","dek":"<base64 nonce|GCM(KEK,DEK)>","ct":"<base64 nonce|GCM(DEK,düz metin)>"}
```

Her değerin kendi rastgele DEK'i vardır; DEK alan KEK'iyle sarılır. Okuma
`secrets.OpenField` ile yapılır: düz metin (geçişten önce yazılmış) olduğu gibi
döner, mühürlü değer anahtarsız asla veri gibi dönmez.

## Yapılandırma

```
FIELD_ENCRYPTION_KEY=<base64 32 bayt>          # KEK kimliği "field"
FIELD_ENCRYPTION_KEYRING=f2027:<base64 32 bayt> # döndürme için ek anahtarlar (isteğe bağlı)
FIELD_ENCRYPTION_ACTIVE_KEY_ID=f2027            # boşsa listedeki son anahtar, o da yoksa "field"
```

- Üretim: `scripts/prod-env.sh upgrade` eksik `FIELD_ENCRYPTION_KEY`'i üretir
  (`deploy/env/schema`, `@norotate`). Geliştirme dışında anahtar boşsa ya da
  `.env.example` değerindeyse arka uç başlamaz.
- Alan anahtarları `APP_ENCRYPTION_KEY` ve kasa anahtarlarıyla aynı olamaz;
  aynıysa sunucu ve worker başlamaz.
- Anahtar tanımlı değilse (yalnızca geliştirme/test) değerler düz metin yazılır
  ve açılışta `field_encryption_disabled` uyarısı loglanır.

## Mevcut veri: geçiş ve geri alma

SQL göçü yoktur; veri `field-crypt` komutuyla taşınır (imajda
`/app/field-crypt`, yerelde `go run ./cmd/field-crypt`). Komut tablo sahibi
rolüyle bağlanır (RLS'ten muaf), her kipte yalnızca değişecek satırları seçer;
yarıda kalırsa yeniden çalıştırılır. Çıktı yalnızca sayılardır.

```
field-crypt -mode status    # kolon × KEK sayıları ("plaintext" = henüz mühürsüz)
field-crypt -mode encrypt   # ileri geçiş: düz metin → mühürlü
field-crypt -mode decrypt   # geri alma: mühürlü → düz metin
field-crypt -mode rotate    # DEK'leri etkin KEK ile yeniden sar
field-crypt -mode encrypt -columns bank_transactions,bank_transaction_sources
```

Sıra: yeni sürümü dağıt (yeni yazılar mühürlü) → `encrypt` → `status` ile
`plaintext` kalmadığını gör. Geri dönmek için önce `decrypt`, sonra eski sürüm:
eski sürüm mühürlü değeri okuyamaz (bugün yalnızca entegratör başlığı okunur).

Boş `{}` değerler içerik taşımadığı için geçişte atlanır. Geçiş `updated_at`
tetikleyicisi yüzünden satırların `updated_at` değerini günceller.

## Anahtar döndürme

1. Yeni anahtarı `FIELD_ENCRYPTION_KEYRING`'e ekle, `FIELD_ENCRYPTION_ACTIVE_KEY_ID`'yi
   ona çevir, yeniden dağıt. Yeni değerler yeni anahtarla sarılır.
2. `field-crypt -mode rotate` çalıştır. Yalnızca DEK yeniden sarılır, şifreli
   veri değişmez.
3. `field-crypt -mode status` eski kimlikte değer göstermediğinde eski anahtarı
   kaldır. `FIELD_ENCRYPTION_KEY` (kimlik `field`) yerinde değiştirilmez; artık
   kullanılmıyorsa da tanımlı kalabilir.

Anahtarı halkada olmayan değer `failed` sayılır, olduğu gibi kalır ve komut 1
ile çıkar.

# Sır kasası (G9, #18)

Entegratör, banka ve program bağlantılarının kimlik bilgileri (`02` §4, `37` §2)
`internal/platform/secrets` paketindeki kasada durur. Bağlantı tablosu yalnızca
opak bir referans tutar: `source_connection.credentials_ref = "vault:<uuid>"`.

## Zarf şifreleme

| Katman | Ne | Nerede |
|---|---|---|
| Veri anahtarı (DEK) | Kayıt başına rastgele 32 bayt, AES-256-GCM | Yalnızca bellekte; sarılmış hâli `secret_records.wrapped_dek` |
| Anahtar şifreleme anahtarı (KEK) | 32 bayt, AES-256-GCM ile DEK'i sarar | Ortam sırrı: `SECRETS_KEYRING`, yoksa `APP_ENCRYPTION_KEY` (kimlik `app`) |
| Şifreli veri | `nonce \| GCM(DEK, düz metin)` | `secret_records.ciphertext` |

Ek doğrulama verisi (AAD) kaydın kimliğini ve amacını (`purpose`) bağlar:
bir satırın şifreli verisi başka satıra kopyalanırsa açılmaz. `Get` amacı da
kontrol eder; `source_connection` için yazılan referans `bank_connection`
olarak okunamaz.

Düz metin hiçbir zaman loglanmaz: kasa `secrets.Secret` döndürür; `fmt`,
`slog` ve JSON bu tipi `[REDACTED]` yazar. Düz metne yalnızca `Reveal()` ile
erişilir.

## Yapılandırma

```
SECRETS_KEYRING=k2026a:<base64 32 bayt>,k2026b:<base64 32 bayt>
SECRETS_ACTIVE_KEY_ID=k2026b
```

- Anahtar üretmek: `openssl rand -base64 32`.
- `SECRETS_KEYRING` boşsa kasa `APP_ENCRYPTION_KEY` ile çalışır. Bu anahtar
  kasada her zaman `app` kimliğiyle okunabilir kalır.
- `SECRETS_ACTIVE_KEY_ID` boşsa listedeki son anahtar etkin olur.

## Anahtar rotasyonu

1. Yeni anahtarı `SECRETS_KEYRING` listesine ekle, `SECRETS_ACTIVE_KEY_ID`'yi
   yeni anahtara çevir, yeniden dağıt. Yeni kayıtlar yeni anahtarla sarılır.
2. `secrets-rotate` komutunu çalıştır (imajda `/app/secrets-rotate`,
   yerelde `go run ./cmd/secrets-rotate`). Eski anahtarla sarılmış her DEK
   yeni anahtarla yeniden sarılır; şifreli veri değişmez, komut tekrar
   çalıştırılabilir. Çıktı yalnızca sayılardır.
3. Çıktıda eski anahtarda kayıt kalmadığında eski anahtarı listeden çıkar.

Anahtarı listede olmayan bir kayıt "failed" sayılır, olduğu gibi bırakılır ve
komut 1 ile çıkar.

## İleride

`secrets.Vault` arayüzü ayrı bir arka uca (HashiCorp Vault, bulut KMS)
taşınabilir. Yeni arka uç kendi referans önekini (`hcv:` gibi) kullanır;
çağıranlar yalnızca `Vault` arayüzüne bağlıdır.

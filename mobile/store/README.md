# Mağaza meta veri taslakları (#183)

**Taslak, gönderilmedi.** Ayrıntı ve açık kararlar: [`docs/07-surum-hazirligi.md`](../../docs/07-surum-hazirligi.md).

| Dosya | Mağaza | Gönderme komutu (yalnızca karar sonrası) |
|---|---|---|
| `store.config.json` | App Store (EAS Metadata biçimi) | `npx eas-cli metadata:push` |
| `play/tr-TR/title.txt` | Google Play, ad (≤ 30) | Play Console ya da `fastlane supply` |
| `play/tr-TR/short-description.txt` | Google Play, kısa açıklama (≤ 80) | |
| `play/tr-TR/full-description.txt` | Google Play, açıklama (≤ 4000) | |
| `play/tr-TR/release-notes.txt` | Google Play, sürüm notu | |

- Yer tutucular: `example.com` adresleri ve `review` bilgileri (inceleme hesabı). Canlıya hazırlıkta gerçek değerlerle değişecek.
- Uzunluk sınırları ve Türkçe liste `src/core/config/release.test.ts` ile denetlenir.
- Ekran görüntüleri henüz yok. Gerekenler: iPhone 6,9" ve 6,5", Android telefon; ekranlar giriş, görevler, fiş çek, fişlerim.

# HAKAN · WhatsApp Web uzantısı

Chrome / Edge uzantısıdır (Manifest V3, TypeScript). WhatsApp Web'de gelen bir görselin ya da PDF'in altına **Gider olarak kaydet**, **Gelir olarak kaydet**, **Belge olarak kaydet** ve **Yabancı fatura olarak kaydet** (#780, ofiste KDV2 taslağı) düğmelerini ekler (issue #388). Kullanıcının kurulum rehberi: [`docs/kullanim/whatsapp-uzantisi.md`](../../docs/kullanim/whatsapp-uzantisi.md).

## Akış

1. Ofis kullanıcısı panelde tek kullanımlık bir eşleştirme kodu alır ve uzantının açılır penceresine yazar. Uzantı bu kodu `POST /v1/auth/whatsapp/pair` ile normal bir oturuma çevirir. Oturum panelde Hesap → Oturumlar listesinde "HAKAN WhatsApp Web uzantısı" adıyla görünür ve oradan kapatılır.
2. Kullanıcı bir düğmeye bastığında uzantı önce sohbetin mükellefini sorar (`chat-lookup`).
   - Eşleme yoksa bir seçici açılır. Seçicide önce iletişim telefonu eşleşen mükellefler önerilir, ayrıca unvan ya da VKN ile aranabilir. Seçim, "Bu sohbet için hatırla" işaretliyse ofis genelinde saklanır (`chat-link`).
3. Belge kaydedilirken mükellefin açık belge talebi varsa uzantı dosyanın hangi talebe yanıt olduğunu sorar.
4. Dosya `POST …/whatsapp/captures` ile gönderilir:
   - Gider ve gelir fiş yakalamaya gider (AI okuma → onay bekleyen taslak).
   - Belge, seçilen talebe eklenir ya da mükellefin eki olarak saklanır.

## Gizlilik

- Sunucuya yalnızca kullanıcının tıkladığı dosya, dosya adı, sohbet kimliği ve sohbet adı gönderilir.
- Mesaj metni okunmaz. `src/dom.ts` yalnızca öznitelik okur; `src/chat.ts` WhatsApp'ın modüllerinden yalnızca aktif sohbetin kimliğini ve adını okur (mesaj listesine dokunmaz); `tests/dom.test.ts` metin okuyucuların kullanılmadığını denetler.
- Uzantı yalnızca `web.whatsapp.com` ile kullanıcının eşleştirmede yazdığı sunucu adresine erişir. Sunucu izni, eşleştirme sırasında yalnızca o adres için istenir (`optional_host_permissions`).
- Belge yakalayıcı (`src/main-world.ts`) yalnızca düğmeye basıldıktan sonraki 20 saniye kurulu kalır ve tek bir indirmeyi yakalar. Kullanıcının kendi indirmelerine dokunmaz.

## Yapı

| Dosya | Görev |
|---|---|
| `src/selectors.ts` | **Bütün WhatsApp Web seçicileri.** Başka dosya sayfa yapısı varsaymaz. |
| `src/dom.ts` | Sohbet kimliği, sohbet adı, kaydedilebilir medya, görüntüleyici. |
| `src/ui.ts` | Düğme şeridi, mükellef / talep seçici, bildirim (gölge DOM). |
| `src/flow.ts` | Bir tıklamanın akışı (mükellef → talep → gönderim). |
| `src/intercept.ts`, `src/main-world.ts` | WhatsApp'ın PDF indirmesini yakalama. |
| `src/chat.ts` | Aktif sohbetin kimliği ve adı, sayfanın kendi dünyasından (postMessage ile istek / yanıt). |
| `scripts/tani.js` | Konsola yapıştırılan, mesaj metni okumayan tanı betiği. |
| `src/content.ts` | İçerik betiği: tarama ve bağlantılar. |
| `src/api.ts`, `src/background.ts` | API istemcisi (hizmet çalışanı), belirteç yenileme. |
| `src/popup.ts`, `static/popup.html` | Eşleştirme penceresi. |
| `tests/fixtures/*.html` | WhatsApp Web DOM örnekleri. |

## Geliştirme

```bash
cd extensions/whatsapp
npm ci
npm test          # vitest + jsdom, fixture'larla
npm run typecheck
npm run build     # dist/
```

`chrome://extensions` (ya da `edge://extensions`) adresinde **Geliştirici modu**nu açın, **Paketlenmemiş öğe yükle** ile `dist/` klasörünü seçin.

`HAKAN_API_URL=https://api.ofisiniz.com npm run build` komutu, açılır penceredeki sunucu adresi alanını bu değerle doldurur.

## Seçiciler kırılırsa

WhatsApp Web DOM'unu habersiz değiştirebilir. Belirtiler: düğmeler görünmez, "Sohbet tanınamadı" ya da "Belgenin indirme düğmesi bulunamadı" uyarıları çıkar. Düzeltme adımları:

1. Kullanıcıdan tanı özeti isteyin: WhatsApp Web'de içinde bir görsel ve bir PDF olan sohbeti açıp F12 → Konsol'a [`scripts/tani.js`](scripts/tani.js) dosyasının tamamını yapıştırsın. Betik mesaj metnini okumaz, rakamları maskeler ve JSON özeti panoya kopyalar: `#main` ve `data-id` örnekleri, başlık / görsel / belge / indirme seçicilerinin sayıları, `data-icon` ve `data-testid` listeleri, son mesaj satırının öznitelik iskeleti ve aktif sohbet erişimcilerinin çalışıp çalışmadığı (kimliğin yalnızca soneki: `@c.us`, `@lid`, `@g.us`).
2. Özete göre `tests/fixtures/` altına yeni bir fixture ekleyin (ör. `chat-2026-11.html`); eskileri silmeyin, eski sürümler için geri dönüş testleri onlarla çalışır. Mesaj metinleri uydurma kalsın.
3. `src/selectors.ts` içindeki seçicileri düzeltin. Başka dosyada seçici yazmayın. Sohbet kimliği DOM'dan değil, WhatsApp'ın modül sisteminden okunuyorsa (güncel sürüm) erişimciler `src/chat.ts` içindedir; tanı özetindeki `sohbetErisimcisi` hangisinin çalıştığını gösterir.
4. `npm test` çalıştırın, ardından `dist/` klasörünü tarayıcıda yeniden yükleyip deneyin.

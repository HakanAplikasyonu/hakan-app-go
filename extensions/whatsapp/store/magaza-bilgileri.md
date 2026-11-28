# Chrome Web Store bilgileri (#780)

Mağaza panelindeki (Chrome Web Store Developer Dashboard) alanlara kopyalanacak metinler. Hepsi koddan çıkarıldı; kod değişince burası da güncellenir. Yükleme adımları: [`docs/whatsapp-magaza-yukleme.md`](../../../docs/whatsapp-magaza-yukleme.md).

| Alan | Değer / dosya |
|---|---|
| Ad | `Easytrade Müşavirim Ofis · WhatsApp` (manifestten gelir) |
| Kısa açıklama | [`kisa-aciklama.txt`](kisa-aciklama.txt) (manifestteki `description` ile aynı, en fazla 132 karakter) |
| Ayrıntılı açıklama | [`ayrintili-aciklama.txt`](ayrintili-aciklama.txt) |
| Kategori | Verimlilik → İş akışı ve planlama (*Productivity → Workflow & Planning*). Bu seçenek yoksa *Productivity → Tools*. |
| Dil | Türkçe |
| Görseller | CI yapıtı `easytrade-whatsapp-magaza-gorselleri`: `ekran-1..4-1280x800.png`, `tanitim-440x280.png`, `simge-128.png` |
| Paket | CI yapıtı `easytrade-whatsapp-magaza`: `magaza-easytrade-whatsapp-<sürüm>.zip` |
| Gizlilik politikası adresi | `https://<panel adresi>/gizlilik/whatsapp-uzantisi` |
| Görünürlük | Liste dışı (*Unlisted*): yalnızca bağlantıyı bilen kurar. Panel bu bağlantıyı gösterir. |

## Tek amaç (Single purpose)

> Uzantının tek amacı, mali müşavirlik ofisi kullanıcısının WhatsApp Web'de kendisine gönderilen bir görseli ya da PDF'i, tıkladığı anda ofisinin Easytrade Müşavirim Ofis hesabına gider, gelir, belge ya da yabancı fatura olarak kaydetmesidir. Araç çubuğu simgesindeki sayaç ve açılır penceredeki özet, aynı hesaptaki bekleyen işleri gösterir ve bu amaca hizmet eder.

## İzin gerekçeleri (Permission justification)

**storage**

> Ofis sunucusunun adresini, eşleştirmeyle alınan oturum yenileme bilgisini ve (kullanıcı yazdıysa) panel adresini bu tarayıcı profilinde saklamak için. Kısa ömürlü erişim bilgisi ve ofis özeti chrome.storage.session'da tutulur, tarayıcı kapanınca silinir. Uzantı bu alanlarda mesaj, kişi ya da dosya saklamaz.

**Konak izni: https://web.whatsapp.com/\***

> İçerik betikleri yalnızca WhatsApp Web'de çalışır: gelen görsel ve PDF'lerin altına kaydetme düğmelerini ekler, kullanıcı bir düğmeye bastığında yalnızca o dosyayı ve aktif sohbetin kimliğini ve adını alır. Mesaj metni okunmaz. Başka hiçbir siteye içerik betiği eklenmez.

**İsteğe bağlı konak izni: https://\<ofis sunucusu\>/\***

> Mağaza paketinde bu izin yalnızca pakete gömülü tek bir sunucu adresini (ofisin Easytrade Müşavirim Ofis sunucusu) kapsar. Kurulumda verilmez; kullanıcı açılır pencerede Eşleştir'e bastığında istenir. Uzantının arka plan hizmet çalışanı eşleştirme, mükellef arama, dosya gönderme ve ofis özeti isteklerini yalnızca bu adrese yapar.

**Uzak kod (Remote code)**

> Hayır. Bütün JavaScript paketin içindedir (`script-src 'self'`); uzaktan betik yüklenmez, `eval` kullanılmaz.

## Gizlilik formu (Data usage)

Uzantının topladığı ya da gönderdiği veriler yalnızca ofisin Easytrade Müşavirim Ofis sunucusuna gider. İşaretlenecek türler:

| Tür | İşaret | Neden |
|---|---|---|
| Kişisel olarak tanımlayıcı bilgiler (*Personally identifiable information*) | Evet | Sohbet kimliği, kişi sohbetinde karşı tarafın telefon numarasını içerir; sohbet adı (kişinin WhatsApp'taki adı) mükellefle eşlemek için gönderilir. Açılır pencere eşleşen kullanıcının adını ve e-postasını gösterir. |
| Sağlık bilgileri | Hayır | — |
| Finansal ve ödeme bilgileri (*Financial and payment information*) | Evet | Kullanıcının tıkladığı fiş ve faturalar tutar ve vergi bilgisi içerir. Kart ya da banka bilgisi istenmez. |
| Kimlik doğrulama bilgileri (*Authentication information*) | Evet | Tek kullanımlık eşleştirme kodu ve sunucunun verdiği oturum bilgileri. Parola istenmez. |
| Kişisel iletişimler (*Personal communications*) | Evet | Kaydedilen dosya bir WhatsApp mesajının ekidir. Yalnızca kullanıcının tıkladığı dosya gönderilir; mesaj metinleri okunmaz. |
| Konum | Hayır | — |
| Web geçmişi | Hayır | — |
| Kullanıcı etkinliği (tıklama, tuş, kaydırma izleme) | Hayır | Yalnızca uzantının kendi düğmelerine basılması işlenir; izleme yapılmaz. |
| Web sitesi içeriği (*Website content*) | Evet | WhatsApp Web'deki görsel ya da PDF (kullanıcı tıkladığında). |

Üç beyanın üçü de işaretlenir:

- Kullanıcı verilerini onaylı kullanım dışında üçüncü taraflara satmıyorum ya da aktarmıyorum.
- Kullanıcı verilerini uzantının tek amacıyla ilgisiz amaçlarla kullanmıyorum ya da aktarmıyorum.
- Kullanıcı verilerini kredi değerliliğini belirlemek ya da borç vermek amacıyla kullanmıyorum ya da aktarmıyorum.

## Koddaki dayanaklar

- Gönderilen istekler: `src/api.ts` (eşleştirme, `chat-lookup`, `chat-link`, `clients?q=`, `document-tasks`, `whatsapp/captures`, özet uçları, `auth/refresh`, `auth/logout`).
- Yalnızca tıklanan dosya: `src/content.ts` (`fileOf`), PDF yakalayıcı `src/intercept.ts` (düğmeden sonra 20 saniye, tek indirme).
- Sohbet kimliği ve adı: `src/chat.ts`, `src/dom.ts` (mesaj metni okunmaz; `tests/dom.test.ts`).
- Saklama: `src/api.ts` (`chrome.storage.local`: `apiBase`, `session`, `webBase`; `chrome.storage.session`: `access`), `src/background.ts` (`summary`).
- Mağaza manifesti: `scripts/store-manifest.mjs` (testi `tests/store.test.ts`).

# Entegratör sözleşme testleri (#196)

Her dış istemci, belgelerdeki örnek istek ve yanıtlardan kurulmuş bir
`httptest` sahte sunucusuna karşı aynı senaryolardan geçer. Testler veritabanı
istemez ve `go test ./...` ile CI'daki `backend` işinde koşar.

| İstemci | Test | Sahte sunucu |
|---|---|---|
| e-Fatura: Nilvera, NES, Muhasebe İzleme, Turkcell, Sovos, İşNet, Trendyol | `internal/modules/entegrator/contract_test.go` | `*_db_test.go` içindeki sahteler (senkron testleriyle ortak) |
| Paraşüt | `internal/modules/sources/parasut/contract_test.go` | `fake_test.go` (JSON:API) |
| KolayBi | `internal/modules/sources/kolaybi/contract_test.go` | Testin kendi sahtesi |
| Easytrade | `internal/modules/sources/easytrade/contract_test.go` | Testin kendi sahtesi |
| VakıfBank | `internal/modules/banking/webservice/vakifbank/contract_test.go` ve `vakifbank_test.go` | Kayıtlı `GetirHareket` yanıtı (`testdata/`) |

Ortak yardımcılar `internal/testutil/contract` paketinde:

- `Fault`: sahtenin önüne konur. İlk N isteğe ya da her isteğe verilen durum
  kodunu döner veya isteği bekletir. `Match` verilirse yalnız eşleşen
  istekleri bozar; örneğin belirteç çağrısını değil, veri çağrısını.
- `Live`: canlı sandbox anahtarlarını ortamdan okur.

## Senaryolar

| Senaryo | e-Fatura (go-ubltr) | Paraşüt / KolayBi / Easytrade | VakıfBank |
|---|---|---|---|
| Sayfalama | Bütün sayfalar `Page.Next` ile gezilir, belge sayısı tutar | Bütün sayfalar okunur | — (pencere başına tek çağrı) |
| Kimlik doğrulama | Reddedilen kimlik `KindAuth`, yeniden denenmez | Reddedilen anahtar `ErrAuth`. Süresi geçmiş belirteç bir kez yenilenir ve çağrı başarılı olur | `vakifbank_test.go`: VBH0012 → `bad_credentials` |
| Hata kodları | 5xx yeniden denenir (3 deneme), sonra yeniden denenebilir hata olarak döner. 429 `KindRateLimit` döner, yeniden denenmez | 5xx bildirilir. Easytrade'de 403 `LICENSE_EXPIRED` → `ErrLicense` | 5xx → `bank_error`, güvenlik duvarı sayfası → `ip_rejected`, VBH kodları |
| Yeniden deneme | Geçici 503 sonrası çağrı başarılı | 429'da artan bekleme ve sonra başarı (Paraşüt, KolayBi). Easytrade'de 429 bildirilir; yeniden denemeyi senkron çalıştırması yapar | — |
| Zaman aşımı | Askıda kalan sunucuda çağrı, çağıranın süresi içinde döner | Aynı | Aynı |
| İndirme | Listelenen belgenin UBL'i iner | — | — |

İzibiz ve Uyumsoft için sahte sunucu yok. Bu ikisi yalnızca canlı test
sistemine karşı koşar (`izibiz_live_test.go`, `uyumsoft_live_test.go`; derleme
etiketleriyle).

## Yeniden deneme kuralı

`entegrator.Client` bütün okuma çağrılarını (listeleme, indirme, durum,
kullanıcı sorgusu) `RetryPolicy` ile sarar. Varsayılan: 3 deneme, 500 ms'den
başlayıp ikiye katlanan bekleme. Yalnızca geçici hatalar yeniden denenir:
entegratöre ulaşılamaması (5xx), bağlantı hatası ve çağıranın süresi bitmemişken
oluşan zaman aşımı.

Hız sınırı (429), kimlik ve istek hataları yeniden denenmez. 429'da bir sonraki
senkron çalıştırması entegratörün aralığına uyar. Bütün çağrılar okuma olduğu
için yeniden deneme bir yan etkiyi tekrarlamaz.

## Canlı sandbox

Varsayılan kapalıdır. Gerçek test hesabı gelince aynı testler okuma
senaryolarıyla (sayfalama, indirme; kaynak programlarda listeleme) canlı
sisteme karşı da koşar:

```bash
CONTRACT_LIVE=1 CONTRACT_NILVERA_API_KEY=... CONTRACT_NILVERA_VKN=... go test -run Contract ./internal/modules/entegrator/
```

| İstemci | Değişkenler (`CONTRACT_` öneki ile) |
|---|---|
| Nilvera, NES | `<AD>_API_KEY`, `<AD>_VKN` |
| Muhasebe İzleme | `MUHASEBEIZLEME_API_KEY`, `MUHASEBEIZLEME_VKN` |
| Turkcell | `TURKCELL_API_KEY` |
| Sovos | `SOVOS_USERNAME`, `_PASSWORD`, `_VKN`, `_POSTBOX_ALIAS`, `_SENDER_ALIAS` |
| İşNet | `ISNET_VKN` (sabit çıkış IP'si gerekir) |
| Trendyol | `TRENDYOL_USERNAME`, `_PASSWORD`, `_USER_ID`, `_COMPANY_ID` |
| Paraşüt | `PARASUT_CLIENT_ID`, `_CLIENT_SECRET`, `_REFRESH_TOKEN`, `_COMPANY_ID` |
| KolayBi | `KOLAYBI_API_KEY`, `KOLAYBI_CHANNEL` |
| Easytrade | `EASYTRADE_EMAIL`, `EASYTRADE_PASSWORD` (isteğe bağlı `_COMPANY_ID`) |
| VakıfBank | `VAKIFBANK_CUSTOMER_NO`, `_USERNAME`, `_PASSWORD` (isteğe bağlı `_ACCOUNT`; sabit çıkış IP'si gerekir) |

Her istemci için isteğe bağlı `_ENDPOINT` test adresini değiştirir. Bir
değişken eksikse canlı alt test atlanır. Hata enjeksiyonu senaryoları her zaman
sahte sunucuya karşı koşar.

Anahtarlar yalnızca ortamdan okunur; repoya ya da CI tanımına yazılmaz. Canlı
çalıştırma yalnızca okuma çağrıları yapar: gönderme, yanıtlama, iptal ve
"alındı" işaretleme istemcide zaten yoktur (`client_test.go`).

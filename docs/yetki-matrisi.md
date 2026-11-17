# Yetki matrisi ve IDOR politikası (#323)

API'nin her ucu, gerçek sunucu üzerinden ve production istek kapsamında (`DB_RLS_REQUEST_SCOPE`) altı kimlikle çağrılır. Her uç için beklenen sonuç testte yazılıdır. Testler: `backend/internal/httpserver/authz_matrix_db_test.go`; uç sınıfları: `authz_table_test.go`.

## Politika: 401, 403, 404

| Durum | Yanıt | Neden |
|---|---|---|
| Belirteç yok, süresi dolmuş ya da oturumu bitmiş | **401** | Kimlik yok. |
| Kapsam dışı: üyesi olunmayan ofis (`X-Organization`), mükellef (`X-Taxpayer`), platform ucu ya da izin yok | **403** | Kimlik belli, erişim yok. |
| Engagement (`/engagements/{engagementUuid}`): bulunamayan, başka ofise ait ya da atanmamış | **403**, hepsi aynı yanıt | Engagement bir kapsamdır. Üç durumun aynı yanıtı vermesi, başka ofisin engagement'ının varlığını sızdırmaz. |
| Kapsam içinde, başka kapsamın kaydı (fatura, fiş, görev, dışa aktarım dosyası, bildirim, oturum, passkey…) | **404** | Arayan için böyle bir kayıt yoktur. 403 dönseydi kimliğin geçerli olduğunu söylerdi. |

Yanıt gövdesi, kapsam dışı ya da bulunamayan bir kayıt için içeriğinden hiçbir şey taşımaz. Testler bunu, başka ofisin ve mükellefin işaretlerini (vergi numarası, unvan, ofis kısa adı) arayarak denetler.

## Kimlikler

| Kimlik | Kim | Hangi kapsamı ister |
|---|---|---|
| `anon` | belirteç yok | – |
| `ownerA` | ofis A (demo) sahibi | ofis A, mükellef X |
| `readerA` | ofis A üyesi; bütün `*.read` ve `*.read_all` izinleri olan özel rol | ofis A, mükellef X |
| `ownerB` | ofis B sahibi | ofis A (yabancı kapsam) |
| `taxX` | mükellef X (Atlas, ofis A'nın müşterisi) | mükellef X |
| `taxY` | mükellef Y (ofis B'nin müşterisi) | mükellef X (yabancı kapsam) |

## Uç sınıfları

| Sınıf | Uçlar | anon | ownerA | readerA | ownerB→A | taxX | taxY→X |
|---|---|---|---|---|---|---|---|
| `azPublic` | sağlık, uygulama ayarı, giriş, parola sıfırlama, davet önizleme ve kabul, yasal belgeler | izin | izin | izin | izin | izin | izin |
| `azPublicOff` | yapılandırmayla kapalı kayıt (`TENANCY_SELF_SERVE=false`) | 403 | – | – | – | – | – |
| `azSelf` | profil, oturumlar, MFA, passkey, bildirimler, push cihazları, arama | 401 | izin | izin | izin | izin | izin |
| `azPlatform` | `/v1/platform/*` | 401 | 403 | 403 | 403 | 403 | 403 |
| `azTenantRead` | ofis okuma | 401 | izin | izin | 403 | 403 | 403 |
| `azTenantReadPriv` | KVKK talepleri, banka şube talimatı | 401 | izin | 403 | 403 | 403 | 403 |
| `azTenantReadAction` | yalnızca okuyan POST'lar: hesap planı denetimi, eksik hesap şablonu, dilekçe önizleme | 401 | izin | izin | 403 | 403 | 403 |
| `azTenantWrite` | ofis yazma | 401 | izin* | 403 | 403 | 403 | 403 |
| `azTenantMember` | ofisten ayrılma | 401 | izin* | izin | 403 | 403 | 403 |
| `azTaxpayerRead` / `azTaxpayerWrite` | `/v1/taxpayer/*` | 401 | 403 | 403 | 403 | izin* | 403 |

\* İzin verilen yazma çağrıları matriste yapılmaz; yan etkileri vardır. Bunları yazma uçları taraması (`TestWriteEndpointsNo5xx`) koşar. "İzin" sonucu: yanıt 401 ya da 403 değil; ayrıca istek geçerli olmayabileceği için 4xx olabilir.

## Testler

- **`TestAuthzMatrix`:** `openapi.yaml`'daki her uç × her kimlik.
  - Tabloda olmayan yeni bir uç testi kırmızı yapar; tablodaki fazla satır da öyle.
- **`TestAuthzIDOR`:**
  - Ofis B kendi kapsamında, kendi engagement'ı altında ofis A'nın kayıt kimlikleriyle bütün uçları çağırır. Mükellef Y de kendi kapsamında X'in kayıtlarıyla çağırır. Hiçbiri 2xx dönmez; okumalar 404 döner; gövdede A ve X'ten iz yoktur.
  - A'nın engagement'ı B'nin başlığı altında 403 döner.
- **`TestAuthzScopedLists`:** ofis B'nin ve mükellef Y'nin kendi listelerinde (kimlik almayan her GET) A ve X'ten bir şey yoktur.
- **`TestAuthzExportScope`:** A ve B ayrı ayrı yevmiye fişi onaylayıp Luca'ya dışa aktarır.
  - Her dosya yalnızca kendi kaydını taşır.
  - Öteki ofis dosyayı kendi engagement'ı üzerinden indiremez (404), öbür ofisin engagement'ı üzerinden de indiremez (403).
- **`TestAuthzSelfRecords`:** bir kullanıcının bildirimi, oturumu, push cihazı ve passkey'i başka bir kullanıcı tarafından okunamaz, işaretlenemez, iptal edilemez, silinemez (404).
- **`TestSessionRevocationEndsAccessTokens`:** çıkış, oturum listesinden iptal, "diğer oturumları kapat" ve parola değişimi o oturumların erişim belirteçlerini hemen keser. Yenileme oturumu sürdürür.

Keşif aracı: `AUTHZ_DISCOVER=1 go test ./internal/httpserver -run 'TestAuthz.*Discover'` gözlenen matrisi `$TMPDIR/authz-matrix.txt` ve `authz-idor.txt` dosyalarına yazar. Yeni bir uç sınıflandırılırken kullanılır.

## Bulunan ve düzeltilen açıklar (#323)

1. **Oturum iptali erişim belirtecini kesmiyordu.**
   - Belirti: çıkış, oturum iptali, "diğer oturumları kapat", parola değişimi ve sıfırlama yalnızca yenileme belirtecini iptal ediyordu. Verilmiş erişim belirteci süresi dolana kadar (`JWT_ACCESS_TTL`, 15 dk) geçerli kalıyordu.
   - Düzeltme:
     - `refresh_tokens.session_uuid`: girişten doğan, yenilemeler boyunca sabit oturum kimliği; erişim belirtecinin `sid`'i.
     - `ended_at`: çıkış, iptal, parola ya da hesap kapatma oturumu bitirir. Kimlik yükleyici her istekte buna bakar.
     - Yenileme yalnızca eski yenileme belirtecini tüketir, oturumu bitirmez.
   - Migration 000091.
2. **Bildirim gelen kutusu ofis ve mükellef kullanıcılarına kapalıydı.** `/v1/notifications*` `notifications.read` istiyordu; bu kullanıcıların genel rolü olmadığı için hepsi 403 alıyordu. Sorgular zaten kullanıcının kendi kimliğiyle. `notifications.read`, `auth.session` gibi oturum tabanına eklendi (#141 ile aynı durum).
3. **Başka kullanıcının bildirimi 403 dönüyordu.** Bu, bildirim kimliğinin geçerli olduğunu söylüyordu. Artık 404 dönüyor (`GET /v1/notifications/{uuid}`, `/action`).

Taramada A'nın kayıt kimliğiyle B'ye 2xx dönen bir uç, başka kapsamın içeriğini taşıyan bir liste ya da dışa aktarım bulunmadı.

## M15 uçları (#453)

M15'te eklenen uçlar (bordro, banka, masraf/Z, Easytrade, rapor, ClickFix, WhatsApp, mutabakat, beyanname, Luca/Zirve aktarımı, co-pilot, ofis mobil, Excel içe/dışa aktarma) `authzTable`'da sınıflandırılmış ve matristen geçiyor. Eksik rol denetimi bulunmadı. Ek testler:

- **Dosya uçları:** `TestAuthzFileExports`, engagement altındaki bütün `.../export`, `.../pdf` ve `.../file` uçları için.
  - Ofis B kendi engagement'ında A'nın kayıt kimlikleriyle 404 alır.
  - Excel ve PDF yanıtları çözülüp aranır; hiçbirinde A'nın verisi (vergi no, unvan, personel, satıcı, ÖKC, banka açıklaması) yoktur.
  - A'nın kendi dosyalarında bu işaretler görünür, yani arama boşa geçmez.
- **Sıkıştırılmış yanıtlar:** `assertNoLeak` Excel (zip) ve PDF gövdelerini çözerek arar. Bu, IDOR ve liste testlerini de kapsar. Önceden sıkıştırılmış içerikte düz metin araması hiçbir şey göremiyordu.
- **Co-pilot planları:** `TestCopilotPlansPerUser`, plan yazanınındır. Aynı ofisin yazma yetkili başka bir üyesi planı okuyamaz, adımını işaretleyemez, iptal edemez (404) ve listesinde görmez. Adımlar istemcide asıl uç çağrılarak çalışır; her uç kendi iznini denetler.
- **Liste sınırları:** `TestListsBounded`, her liste için:
  - `limit=1`'e uyar ya da `boundedLists`'te gerekçesiyle (katalog, sunucu tarafı sınır, dönem başına) anılır;
  - en çok 500 kayıt döner;
  - hatalı `limit` ya da `offset` 5xx vermez.

  Banka karşı taraf takma adları listesi bu kapsamda sayfalı yapıldı.
- **İçe aktarma:** `TestM15ImportsHostileFiles`, masraf, Z, bordro, BA/BS, banka ekstresi ve genel Excel içe aktarma için bozuk ve sınır değerli dosyalar dener:
  - Excel olmayan, boş, Excel olmayan zip, boş çalışma kitabı, başlıksız, yalnız başlık, hatalı değerler, BOM'lu CSV, 5 001 satır, 21 MB.
  - Hiçbiri 5xx vermez; reddedilen dosya satır yazmaz.

  `TestExpenseImportRoundTrip` ve `TestRetailSaleImportRoundTrip`: dışa aktarılan dosya yeniden yüklenince satır eklenmez, hatalı satır numarasıyla bildirilir, ofis B'nin içe aktarması A'yı etkilemez.
- **Migration:** `TestMigrationsUpDownUp` (bütün migration'lar up, down, up; şema birebir) ve `TestM15ForeignKeysIndexed` (M15 yabancı anahtarlarının indeksi; migration 000115).

Bulunan ve düzeltilenler:

1. **Excel dışa aktarımı yeniden yüklenince kayıtlar ikileniyordu.** Masraf ve Z raporu içe aktarması yinelemeyi yalnızca daha önce Excel'den gelmiş satırlarda tanıyordu. Fiş fotoğrafından, Easytrade'den ya da demo verisinden gelen kayıtlar, dışa aktarılan dosya geri yüklendiğinde ikinci kez ekleniyordu. Artık eşleşen kayıt hangi kaynaktan gelmiş olursa olsun atlanıyor:
   - masrafta tarih, satıcı, VKN, fiş no ve toplam;
   - Z raporunda tarih, ÖKC ve Z no.
2. **Excel olmayan bir zip bordro ve BA/BS içe aktarmasında 500 veriyordu.** Artık okunamayan dosya 4xx dönüyor.

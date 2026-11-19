# Çapraz denetim: arayüzde karşılığı olmayan yetenekler (#517)

| | |
|---|---|
| Soru | Backend'in sunduğu her yetenek arayüzde, doğru yerde (ofis / mükellef düzeyi) ve doğru role açık mı? |
| Tarih | 2026-11-19 |
| Kapsam | `backend/docs/openapi.yaml` (524 uç), web (`frontend/src`), `mobile/`, `mobile-office/`, `desktop/`, `extensions/whatsapp`, `backend/cmd/demo-seed` |
| Uygulama | #518 (bu belgedeki sıraya göre) |
| Kod değişikliği | Yok |

## Yöntem

1. OpenAPI'deki her uç, kaynağına göre gruplandı: kapsam (`tenant`, `tenant/eng` = `/v1/tenant/engagements/{engagementUuid}/…`, `taxpayer`, `platform` …) + ilk sabit parça (ör. `bank-connections`). 161 kaynak çıktı.
2. Her kaynağın yolu, istemcilerin kaynak kodunda (üretilmiş tip dosyaları `generated/`, `schema.d.ts` hariç) arandı. Web servisleri yolu `${eng(engagement)}/account-rules` gibi parçalı kurduğu için arama son parçaya göre yapıldı.
3. Hiçbir istemcide geçmeyen kaynaklar tek tek elle doğrulandı (servis dosyası, sayfa, sekme listesi, `engagement-layout.tsx`, `config/nav.ts`, `tenant-settings-page.tsx`).
4. Yetki ekseni için backend'de geçen `tenant.*` izin anahtarları web'in izin kataloğuyla (`config/permissions.ts` ve kullanımlar) karşılaştırıldı.
5. Demo verisi için eksik ekranların tabloları `backend/cmd/demo-seed` içinde arandı.

Sınırlar: Uygulama çalıştırılmadı, ekran görüntüsü alınmadı; tespitler kaynak koda dayanır. Rota bağlantıları `make demo-big` ile açılan ortama göredir. "Doğrulanmalı" işaretli satırlar #518'de ekranda teyit edilmeli.

## Özet

- **En büyük boşluk mükellef düzeyindeki bağlantı ayarları.** Mükellef detayında (`/t/{slug}/engagements/{uuid}`) bir "Bağlantılar / Entegrasyonlar" sekmesi yok. Sekmeler: Genel bakış, Faturalar, Uyarılar, İnceleme, Banka, Bordro, Easytrade, Masraflar, Z raporları, Mutabakat, Yevmiye, Belgeler, Yükleme, Beyanname, Dilekçe, Rapor.
- **e-Fatura entegratörü bağlantısı hiçbir yerden kurulamıyor.** `backend/internal/modules/entegrator` dokuz sağlayıcıyla (İzibiz, İşNet, NES, Nilvera, Sovos, Turkcell, Uyumsoft, Muhasebe İzleme, Trendyol) senkron yapıyor; ama bağlantı oluşturan, düzenleyen ya da durumunu gösteren bir HTTP ucu yok. Bağlantılar yalnızca demo verisi ve testlerle oluşuyor.
- Banka web servisi, Paraşüt, KolayBi, genel Excel içe aktarma, KVKK talepleri, kilitli dönem değişiklik talepleri, dönem özeti ve araç listesi: **backend ucu var, hiçbir istemcide ekranı yok.**
- Mükellefin fiş düzeltme talebi (`POST /v1/taxpayer/receipts/{uuid}/corrections`) ne web mükellef panelinde ne mükellef mobil uygulamasında kullanılıyor. Ofis tarafında da bunları karara bağlayan ekran (`change-requests`) yok. Akış iki uçta da kopuk.

## Bulgular

Önem: **P0** ürünün temel vaadini engeller · **P1** var olan bir yetenek kullanılamıyor · **P2** iyileştirme. Tahmini iş: S (≤½ gün), M (1–2 gün), L (3+ gün).

### A. Ofis düzeyi ↔ mükellef düzeyi (bağlantı ve ayarlar)

| # | Eksen | Nerede | Beklenen | Mevcut durum | Önem | İş |
|---|---|---|---|---|---|---|
| A1 ✅ | Ofis↔mükellef | Mükellef detayı | e-Fatura/e-Arşiv entegratörü (QNB EDM, İzibiz, İşNet, Nilvera, Uyumsoft …) mükellef bazında bağlanır, test edilir, son senkron ve hata görünür | Backend senkron var (`modules/entegrator`, `queue/entegrator_sync.go`); **API ucu ve arayüz yok**. `source_connections` yalnızca demo/testte oluşuyor. **Yapıldı (#518):** `/v1/tenant/einvoice-integrators` ve `/einvoice-connections` uçları (oluştur, kimlik kasaya, ilk çekim sabit IP'li işçide, şimdi çek, son çalışma); Bağlantılar sekmesinde entegratör kartı; demo verisinde bağlı İzibiz. Kalan: entegratör senkronunu düzenli çalıştıran zamanlayıcı yok (bugün yalnızca kurulum ve "şimdi çek" kuyruğa atar) | P0 | L (API + ekran) |
| A2 ✅ | Backend↔arayüz | Mükellef detayı → Bağlantılar sekmesi | Banka web servis bağlantısı: banka seçimi (`GET /v1/tenant/bank-services`), şube talimatı, kimlik bilgisi, senkron | 7 uç var (`/bank-connections`, `/instruction`, `/submitted`, `/credentials`, `/sync`); **ekran yok**. Banka sekmesi yalnızca ekstre yükleme ve eşleştirme. **Yapıldı (#518):** Bağlantılar sekmesinde dört adımlı kurulum (talimat PDF, şubeye teslim, servis kullanıcısı, şimdi çek); demo verisinde şubede bekleyen Vakıfbank bağlantısı | P0 | M |
| A3 ✅ | Backend↔arayüz | Mükellef detayı | Paraşüt bağlantısı (OAuth yetkilendirme, şirket seçimi, senkron) | 6 uç var (`/parasut-connections…`), ekran yok. **Yapıldı (#518):** Bağlantılar sekmesinde "Ön muhasebe programı" kartı (onay, şirket seçimi, son 30 günü çek, son çekimin sayıları) ve `/parasut/callback` dönüş sayfası (`PARASUT_REDIRECT_URL` bu yolu göstermeli); demo verisinde bağlı Paraşüt | P1 | M |
| A4 ✅ | Backend↔arayüz | Mükellef detayı | KolayBi bağlantısı (şirket seçimi, senkron) | 4 uç var (`/kolaybi-connections…`), ekran yok. **Yapıldı (#518):** aynı kartta API anahtarıyla bağlanma, şirket seçimi, yeniden bağlanma ve çekim | P1 | S–M |
| A5 | Ofis↔mükellef | Mükellef detayı → İnceleme | Mükellefe özel inceleme eşikleri (ofis varsayılanını ezme) | `GET/PUT/DELETE /review-settings` var; web yalnızca ofis düzeyini gösteriyor (`AiApprovalSettings`, `tenant/review/settings`) | P1 | S |
| A6 | Ofis↔mükellef | Ofis ayarları → Hesap kuralları | Ofis düzeyi hesap kuralları (bütün mükelleflere şablon) | `/v1/tenant/account-rules` (6 uç) kullanılmıyor; web yalnızca mükellef düzeyini kullanıyor (`account-memory.service.ts`) | P2 | M |
| A7 | Bilinçli olarak yok | — | e-Tebligat, Dijital Vergi Dairesi, SGK | `docs/04`, `docs/05` kararı: ilk dilimde otomatik bağlantı yok. Arayüzde de yok; tutarlı | — | — |
| A8 | Ofis↔mükellef | Mükellef detayı | Bildirim tercihleri mükellef bazında | Yalnızca kullanıcı düzeyi (`/v1/notification-preferences`); mükellef bazında backend ucu da yok. Ürün kararı gerekir | P2 | — |

Var ve doğru yerde olanlar (eksik değil): Easytrade bağlantısı (mükellef sekmesi), muhasebe programı aktarım ayarı (`journal-export-settings`, ofis + mükellef), WhatsApp kanalı (ofis sayfası + mükellef ucu), masaüstü ajanları (ofis ayarları + Ajanlar), uyarı kuralları (ofis + mükellef).

### B. Backend ↔ arayüz (ofis paneli, diğer yetenekler)

| # | Nerede | Beklenen | Mevcut durum | Önem | İş |
|---|---|---|---|---|---|
| B1 | Mükellef detayı → Belgeler / Yükleme | Genel Excel içe aktarma sihirbazı: yükle → sütun eşle → doğrula → satır düzelt → işle / geri al; şablonlar | 12 uç var (`/excel-imports…`, `/v1/tenant/excel-import-templates`, `/excel-import-data-types`); ekran yok. Demo verisi var (`demo-seed/excelimport.go`) | P1 | L |
| B2 ✅ | Mükellef detayı → dönem şeridi | Kilitli döneme gelen değişiklik talepleri: liste, uygula, reddet | `/change-requests` (4 uç) ekransız; kilitli döneme düşen kayıtlar sessizce bekliyor. **Yapıldı (#518):** dönem şeridinin altında açık talep paneli (alan farkı eski→yeni, kaynak, faturayı aç, uygula/reddet; `tenant.periods.read` / `tenant.change_requests.decide`). Kalan: demo verisinde talep yok | P1 | M |
| B3 | Mükellef detayı (yeni sekme ya da Ayarlar) | KVKK ilgili kişi talepleri: oluştur, onayla/reddet, dışa aktar | `/privacy-requests` (6 uç) ekransız; yetki matrisi bunu `azTenantReadPriv` olarak ayırmış | P1 | M |
| B4 | Mükellef detayı → Rapor | Yapay zekâ dönem özeti: taslak, düzenle, onayla | `/period-summaries` (5 uç) ekransız; demo verisinde yalnızca dolaylı geçiyor | P2 | M |
| B5 | Mükellef detayı → Uyarılar | "Kontrolleri çalıştır" düğmesi | `POST /controls/run` ekransız | P2 | S |
| B6 | Mükellef detayı → Banka | "Faturalarla eşleştir" düğmesi | `POST /bank-matching` ekransız | P2 | S |
| B7 | Mükellef detayı → Masraflar | Araç listesi (masrafta araç seçimi) | `/vehicles` (3 uç) ekransız | P2 | S |
| B8 | Ofis ayarları | Yapay zekâ kullanım/maliyet raporu ve gizlilik ayarı | `/v1/tenant/ai/usage`, `/ai/privacy` ekransız | P2 | S |
| B9 | Ofis ayarları | Ofisin abonelik / fatura durumu | `GET /v1/tenant/billing` ekransız | P2 | S |

### C. Mükellef paneli ve mükellef mobil uygulaması

| # | Nerede | Beklenen | Mevcut durum | Önem | İş |
|---|---|---|---|---|---|
| C1 ◐ | `/m/{taxpayer}/receipts` ve mobil | Fişe düzeltme talebi gönderme | `POST /v1/taxpayer/receipts/{uuid}/corrections` iki istemcide de yok (ofis tarafı B2 ile birlikte). **Web yapıldı (#518):** fiş listesinde "Düzeltme öner" penceresi (satıcı, tarih, toplam, KDV, fiş no, not). Kalan: mobil uygulama | P1 | S |
| C2 | `mobile/` | Rapor, yükleme geçmişi, geçmiş paylaşımı (history-shares) | Web mükellef panelinde var (`/m/.../report`, `/uploads`), mobilde yok | P2 | M |
| C3 | `mobile/`, `mobile-office/` | Çıkışta push cihaz kaydını silme | `DELETE /v1/push-devices/{token}` hiçbir istemcide yok; çıkıştan sonra bildirim gelmeye devam edebilir (doğrulanmalı) | P2 | S |

### D. Platform yönetim paneli

| # | Nerede | Beklenen | Mevcut durum | Önem | İş |
|---|---|---|---|---|---|
| D1 | `/platform/organizations/{uuid}` | Abonelik planı, ofis faturalandırması, dönem ekstresi | `/v1/platform/billing/plans`, `/organizations/{uuid}/billing…` ekransız | P2 | M |
| D2 | `/platform` | Mükellef kayıtları (platform düzeyi liste/düzenleme) | `/v1/platform/taxpayers` (4 uç) ekransız | P2 | S |
| D3 | `/platform/settings` | Yasal belge sürümü yayımlama (KVKK, sözleşme) | `/v1/platform/legal/documents` (3 uç) ekransız; şu an yalnızca seed ile | P2 | S |
| D4 | `/platform/settings/system` | Sabit çıkış IP'si durumu (bankalar IP beyaz listesi ister) | `GET /v1/platform/egress` ekransız; A2 ile birlikte gerekli | P2 | S |

### E. Rol ve yetki

| # | Bulgu | Önem | İş |
|---|---|---|---|
| E1 ✅ | Backend'in kullandığı şu izinler web'in izin kataloğunda hiç geçmiyor: `tenant.periods.lock`, `tenant.periods.read`, `tenant.warnings.read`, `tenant.warning_rules.write`, `tenant.doc_tasks.read`, `tenant.vat.read/write`, `tenant.change_requests.decide`, `tenant.excel_imports.*`, `tenant.summaries.*`, `tenant.billing.write`. Sonuç: ilgili düğmeler (ör. dönem kilitle) yetkisiz kullanıcıya da görünüyor, tıklayınca 403 alıyor. Bu yetkiler rol düzenleme ekranında görünüyor mu, ayrıca doğrulanmalı **Yapıldı (#518):** eksik 15 kiracı izni web kataloğuna eklendi; bir test backend (`rbac.go`) ile kataloğu karşılaştırır, yeni izin eklenip katalog unutulursa kırılır. Dönem kilitleme, uyarı kuralı, KDV ve belge görevi yazma düğmeleri web'de henüz yok; ekranları yapılınca katalogdaki izinle gizlenir | P1 | M |
| E2 ✅ | Sekmelerin çoğu (`engagement-layout.tsx`) yalnızca Bordro, Yevmiye ve Beyanname için izin kontrolü yapıyor. Uyarılar, Banka, Masraflar, Mutabakat gibi sekmeler izinsiz rolde açılıp boş/hata durumuna düşüyor (doğrulanmalı) **Yapıldı (#518):** Faturalar, Uyarılar, İnceleme, Banka, Masraflar, Z raporları, Mutabakat ve Belgeler sekmeleri de ana listelerinin okuma iznine bağlandı (`visibleTabs`) | P2 | S |

### F. Platformlar arası (bilinçli kapsam farkları dışında)

- **Ofis mobil** (`mobile-office/`) onay kuyruğu, uyarılar, belge görevleri, bordro, mizan, arşiv, co-pilot ve bildirimleri kapsıyor. Banka, masraf, mutabakat, beyanname, yevmiye mobilde yok. Bunlar masaüstü iş akışı sayılıp bilinçli olarak dışarıda bırakılmış görünüyor; eksik sayılmadı.
- **Masaüstü** (`desktop/`) ajan kaydı, heartbeat ve aktarım görevlerini kullanıyor. Eksik bulunmadı.
- **WhatsApp eklentisi** giriş, eşleştirme, sohbet arama, mükellef listesi ve UBL/belge yüklemeyi kullanıyor. Eksik bulunmadı.

### G. Demo verisi

`make demo-big` verisinde şu ekranlar boş kalacak (ekran yapılınca veri de eklenmeli): banka web servis bağlantısı (A2), Paraşüt/KolayBi (A3, A4), KVKK talepleri (B3), değişiklik talepleri (B2), mükellef inceleme eşiği (A5), hesap kuralları (A6, ofis düzeyi). e-Fatura entegratörü bağlantısı yalnızca Easytrade için demoda var (`demo-seed/easytrade.go`).

### H. Kayıtlar arası

#469 kapsamı dışında yeni bir kopuk bağlantı bulunmadı. Bu eksen kod okumasıyla sınırlı kaldı; #518 sırasında ekranlar gezilirken yeniden bakılmalı.

## Önerilen uygulama sırası (#518)

Durum: 1–6 yapıldı (D4 ertelendi: platform paneli P2; C1 mobil kaldı). Sıradaki: 7.

1. ✅ **Mükellef detayına "Bağlantılar" sekmesi** iskeleti: bütün mükellef düzeyi bağlantıları (entegratör, banka, Easytrade, Paraşüt, KolayBi, WhatsApp, muhasebe aktarımı) tek yerde, durum rozeti ve son senkron saatiyle. Easytrade ve aktarım ayarı buraya bağlanır.
2. ✅ **A2 banka web servis bağlantısı** (API hazır) + D4 çıkış IP'si bilgisi.
3. **A1 e-Fatura entegratörü**: önce backend uçları (oluştur, kimlik bilgisini kasaya yaz, test et, senkron başlat, durum), sonra sekmeye ekran.
4. **A3 Paraşüt, A4 KolayBi** (API hazır).
5. ✅ **B2 + C1 değişiklik/düzeltme talepleri** (iki uçlu akış birlikte). (C1 mobil kaldı.)
6. ✅ **E1 izin kataloğu** eşitlemesi ve düğme/sekme gizleme (E2).
7. **B3 KVKK talepleri.**
8. **B1 genel Excel içe aktarma sihirbazı.**
9. **A5 inceleme eşiği**, ardından B4–B9, C2–C3, D1–D3, A6 (P2'ler).

Her adımda demo verisi (G) aynı PR'da doldurulur.

## Karar gerekiyor

- **A1** için backend'de uç yok: #518 kapsamı yalnızca arayüz sanılıyorsa, bu madde ayrı bir backend issue'su olarak açılmalı mı? Öneri: #518'in içinde, ayrı PR olarak.
- **A8** mükellef bazında bildirim tercihi ürün kararı ister; şimdilik kapsam dışı bırakıldı.

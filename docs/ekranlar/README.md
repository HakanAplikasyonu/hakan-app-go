# Arayüz v1 ekran galerisi (M14)

Yeni arayüzün proje sahibine gösterilecek ekran görüntüleri (Refs #324).

**Nasıl alındı**
- Yığın yerelde `scripts/e2e-up.sh` ile kuruldu: Postgres ve diğer compose servisleri, sıfırdan migration, `demo-seed -reset`, Go API ve `next build && next start`.
- Ekranlar demo verisine ek olarak e2e akışlarının ürettiği verilerle dolduruldu:
  - duman akışı: UBL yükleme, banka ekstresi ve eşleme, yevmiye ve çıktı;
  - mizan hafızası senaryosu: Oğuz Gıda için Luca mizanı, düzeltmeler, kural ve ikinci ay;
  - mükellef onay akışıyla eklenen 3 mükellef ve onay bekleyen 2 mükellef;
  - API'den açılan 3 belge talebi ve kaydedilen 3 masaüstü ajanı.
- Hiçbir öğe maskelenmedi.
- Ofis paneli ve mükellef portalı Playwright ile çekildi:
  - varsayılan: `m3-light` tema, masaüstü 1440×900;
  - her yüzeyden 2 ekran `m3-dark` ve `n3-light` temada;
  - 3 ekran telefon boyutunda (390×844).
- Boş ve hata durumları, `e2e/tests/gorsel.spec.ts`'teki gibi inceleme kuyruğu ucunun yanıtı taklit edilerek çekildi.
- Masaüstü ajanı `desktop/ui` dosyalarından, `desktop/tests/ui` düzeneğiyle (sahte Tauri IPC, `mock-tauri.js`) çekildi:
  - pencere boyutu 1140×760;
  - "dar" kırılım 480×800. Pencerenin en küçük genişliği 720 olduğu için telefon boyutu yerine bu kırılım kullanıldı.
- Tepsi simgeleri `desktop/src-tauri/src/tray.rs` içindeki `render()` işlevinin olduğu gibi derlenmesiyle üretildi.
- Mobil uygulamalar `npx expo export --platform web` ile derlendi ve 390×844'te çekildi.
  - Go API CORS yanıtı vermediği için Expo web'in API istekleri Playwright'ta yakalanıp CORS başlıklarıyla geçirildi. Ürün kodu değişmedi.
- Görüntüler pngquant ile sıkıştırıldı; her biri 400 KB'tan küçük.

**Ortak tablo çekirdeği güncellemesi (#381).** Ofis listeleri ortak tablo çekirdeğine taşındıktan sonra şu ekranlar aynı yöntemle yeniden çekildi (m3-light 1440×900; tema ve telefon varyantları kendi boyutlarında): 02, 05, 07, 08, 09, 14, 15, 18, 20, 22, 24. Bu çekimde pngquant yoktu; dosyalar sıkıştırılmadı ama yine 400 KB'tan küçük. 10 (mizan hafızası) ve 11 (yevmiye) yeniden çekilmedi: o ekranların örnek verisi mizan hafızası senaryosunun tamamından geliyor ve senaryo bu çekimde yarıda kaldı (#381'in PR'ında açıklandı). İki tablo da aynı çekirdeğe taşındı.
Mükellef portalının faturalar ekranı (portal-04) da çekirdeğe taşındıktan sonra aynı yöntemle yeniden çekildi.

## Ofis web paneli

| # | Dosya | Açıklama |
|---|---|---|
| 1 | [ofis-01-giris.png](ofis-01-giris.png) | Giriş ekranı. |
| 2 | [ofis-02-genel-bakis.png](ofis-02-genel-bakis.png) | Genel bakış: dönem tacı, kuyruk / uyarı / belge görevi sayaçları, işi geride olan mükellefler, KDV tahmini. |
| 3 | [ofis-03-inceleme-kuyrugu.png](ofis-03-inceleme-kuyrugu.png) | İnceleme kuyruğu, bir öneri (Ofis Koltuğu → 600) açık: kaynak satır, "Neden bu kod?", onayla / düzelt / kural yap / reddet. |
| 4 | [ofis-04-uyarilar.png](ofis-04-uyarilar.png) | Uyarılar: engelleyici GİB Schematron uyarısı ve faturası olmayan tahsilat. |
| 5 | [ofis-05-mukellef-listesi.png](ofis-05-mukellef-listesi.png) | Mükellef listesi. |
| 6 | [ofis-06-mukellef-detayi.png](ofis-06-mukellef-detayi.png) | Mükellef detayı (Atlas Gıda): dönem şeridi, 2026-10 hazırlığı, açık belge istekleri. |
| 7 | [ofis-07-faturalar.png](ofis-07-faturalar.png) | Faturalar listesi (Atlas Gıda). |
| 8 | [ofis-08-fatura-cekmecesi.png](ofis-08-fatura-cekmecesi.png) | Fatura çekmecesi: tutar şelalesi, satırlar, "Mükelleften düzeltme iste". |
| 9 | [ofis-09-banka.png](ofis-09-banka.png) | Banka: Ziraat ekstresi hareketleri ve eşleme durumları. |
| 10 | [ofis-10-mizan-hafizasi.png](ofis-10-mizan-hafizasi.png) | Mizan hafızası (Oğuz Gıda): etkin kural, soru sorulmadan kodlanan kalemler, Luca hesap planı. |
| 11 | [ofis-11-yevmiye-ve-cikti.png](ofis-11-yevmiye-ve-cikti.png) | Yevmiye ve çıktı: yeni fiş, aktarılmış fişler, Luca / Zirve / Excel çıktısı. |
| 12 | [ofis-12-ayarlar-yapay-zeka-esik.png](ofis-12-ayarlar-yapay-zeka-esik.png) | Ayarlar → Yapay zekâ ve onay: otomatik onay eşiği ve mizan hafızasının kural adayı eşiği. |
| 13 | [ofis-13-kurulum-sihirbazi.png](ofis-13-kurulum-sihirbazi.png) | Ofis kurulum sihirbazı (6 adım). |
| 14 | [ofis-14-masaustu-ajanlari.png](ofis-14-masaustu-ajanlari.png) | Masaüstü ajanları listesi. |
| 15 | [ofis-15-komuta-merkezi.png](ofis-15-komuta-merkezi.png) | Komuta merkezi. |
| 16 | [ofis-16-bos-durum.png](ofis-16-bos-durum.png) | Boş durum örneği: bekleyen öneri yok. |
| 17 | [ofis-17-hata-durumu.png](ofis-17-hata-durumu.png) | Hata durumu örneği: öneriler yüklenemedi. |
| 18 | [ofis-18-genel-bakis-m3-dark.png](ofis-18-genel-bakis-m3-dark.png) | Genel bakış, m3-dark. |
| 19 | [ofis-19-inceleme-kuyrugu-m3-dark.png](ofis-19-inceleme-kuyrugu-m3-dark.png) | İnceleme kuyruğu, m3-dark. |
| 20 | [ofis-20-genel-bakis-n3-light.png](ofis-20-genel-bakis-n3-light.png) | Genel bakış, n3-light. |
| 21 | [ofis-21-inceleme-kuyrugu-n3-light.png](ofis-21-inceleme-kuyrugu-n3-light.png) | İnceleme kuyruğu, n3-light. |
| 22 | [ofis-22-genel-bakis-telefon.png](ofis-22-genel-bakis-telefon.png) | Genel bakış, telefon (390×844). |
| 23 | [ofis-23-inceleme-kuyrugu-telefon.png](ofis-23-inceleme-kuyrugu-telefon.png) | İnceleme kuyruğu, telefon. |
| 24 | [ofis-24-faturalar-telefon.png](ofis-24-faturalar-telefon.png) | Faturalar, telefon. |
| 25 | [ofis-25-giris-m3-dark.png](ofis-25-giris-m3-dark.png) | Giriş ekranı, m3-dark. |

## Mükellef web portalı

| # | Dosya | Açıklama |
|---|---|---|
| 1 | [portal-01-ana-sayfa.png](portal-01-ana-sayfa.png) | Ana sayfa: müşavirin istedikleri, sürükle-bırak yükleme alanı, geçmiş veri paylaşımı. |
| 2 | [portal-02-belge-yukleme.png](portal-02-belge-yukleme.png) | Belge yükleme. |
| 3 | [portal-03-belge-talepleri.png](portal-03-belge-talepleri.png) | Belge talepleri (İstekler): bekleyen 3 talep. |
| 4 | [portal-04-faturalar.png](portal-04-faturalar.png) | Mükellefin faturaları. |
| 5 | [portal-05-ana-sayfa-m3-dark.png](portal-05-ana-sayfa-m3-dark.png) | Ana sayfa, m3-dark. |
| 6 | [portal-06-belge-talepleri-m3-dark.png](portal-06-belge-talepleri-m3-dark.png) | Belge talepleri, m3-dark. |
| 7 | [portal-07-ana-sayfa-n3-light.png](portal-07-ana-sayfa-n3-light.png) | Ana sayfa, n3-light. |
| 8 | [portal-08-belge-talepleri-n3-light.png](portal-08-belge-talepleri-n3-light.png) | Belge talepleri, n3-light. |
| 9 | [portal-09-ana-sayfa-telefon.png](portal-09-ana-sayfa-telefon.png) | Ana sayfa, telefon. |
| 10 | [portal-10-belge-yukleme-telefon.png](portal-10-belge-yukleme-telefon.png) | Belge yükleme, telefon. |
| 11 | [portal-11-belge-talepleri-telefon.png](portal-11-belge-talepleri-telefon.png) | Belge talepleri, telefon. |

**Bildirimler:** Mükellef web portalında ayrı bir bildirim ekranı yok. Yeni talepler üst menüdeki "İstekler" rozetinde (3) görünür. Bildirim listesi mükellef mobil uygulamasında: `mobil-mukellef-05-bildirimler.png`.

## Masaüstü ajanı

| # | Dosya | Açıklama |
|---|---|---|
| 1 | [masaustu-01-kurulum-1-giris.png](masaustu-01-kurulum-1-giris.png) | Kurulum 1/3: HAKAN hesabıyla giriş. |
| 2 | [masaustu-02-kurulum-2-bilgisayar.png](masaustu-02-kurulum-2-bilgisayar.png) | Kurulum 2/3: ofis seçimi ve bilgisayarı tanıtma. |
| 3 | [masaustu-03-kurulum-3-ilk-klasor.png](masaustu-03-kurulum-3-ilk-klasor.png) | Kurulum 3/3: ilk klasörü mükellefe bağlama. |
| 4 | [masaustu-04-klasorler.png](masaustu-04-klasorler.png) | İzlenen klasörler: biri sorunlu üç klasör. |
| 5 | [masaustu-05-gecmis.png](masaustu-05-gecmis.png) | Gönderim geçmişi. |
| 6 | [masaustu-06-sorunlar.png](masaustu-06-sorunlar.png) | Sorunlar: yeniden denenecek ve kalıcı hatalar. |
| 7 | [masaustu-07-ayarlar.png](masaustu-07-ayarlar.png) | Ayarlar. |
| 8 | [masaustu-08-tepsi-durumlari.png](masaustu-08-tepsi-durumlari.png) | Tepsi simgesinin 5 durumu, açık ve koyu menü çubuğunda: bağlı, gönderiyor, duraklatıldı, sorun var, güncelleme hazır. |
| 9 | [masaustu-09-klasorler-m3-dark.png](masaustu-09-klasorler-m3-dark.png) | Klasörler, m3-dark. |
| 10 | [masaustu-10-sorunlar-m3-dark.png](masaustu-10-sorunlar-m3-dark.png) | Sorunlar, m3-dark. |
| 11 | [masaustu-11-klasorler-n3-light.png](masaustu-11-klasorler-n3-light.png) | Klasörler, n3-light. |
| 12 | [masaustu-12-sorunlar-n3-light.png](masaustu-12-sorunlar-n3-light.png) | Sorunlar, n3-light. |
| 13 | [masaustu-13-klasorler-dar.png](masaustu-13-klasorler-dar.png) | Klasörler, dar pencere (480×800, üst sekme şeridi). |
| 14 | [masaustu-14-gecmis-dar.png](masaustu-14-gecmis-dar.png) | Geçmiş, dar pencere. |
| 15 | [masaustu-15-sorunlar-dar.png](masaustu-15-sorunlar-dar.png) | Sorunlar, dar pencere. |

## Mükellef mobil uygulaması (`mobile/`)

| # | Dosya | Açıklama |
|---|---|---|
| 1 | [mobil-mukellef-01-kvkk-onay.png](mobil-mukellef-01-kvkk-onay.png) | İlk açılış: KVKK onayı. |
| 2 | [mobil-mukellef-02-ana-sayfa.png](mobil-mukellef-02-ana-sayfa.png) | Ana sayfa: fiş / Z raporu çek, müşavirin istedikleri. |
| 3 | [mobil-mukellef-03-istekler.png](mobil-mukellef-03-istekler.png) | İstekler (belge talepleri). |
| 4 | [mobil-mukellef-04-gecmis-faturalar.png](mobil-mukellef-04-gecmis-faturalar.png) | Geçmiş → Faturalar. |
| 5 | [mobil-mukellef-05-bildirimler.png](mobil-mukellef-05-bildirimler.png) | Bildirimler: yeni belge talepleri. |
| 6 | [mobil-mukellef-06-profil.png](mobil-mukellef-06-profil.png) | Profil ve görünüm (M3 / N3, açık / koyu). |

## Ofis mobil uygulaması (`mobile-office/`)

| # | Dosya | Açıklama |
|---|---|---|
| 1 | [mobil-ofis-01-pano.png](mobil-ofis-01-pano.png) | Pano: dönem durumu, onay kuyruğu, belge görevi, engel. |
| 2 | [mobil-ofis-02-onay-kuyrugu.png](mobil-ofis-02-onay-kuyrugu.png) | Onay kuyruğu: kaydırmalı öneri kartı. |
| 3 | [mobil-ofis-03-mukellefler.png](mobil-ofis-03-mukellefler.png) | Mükellefler ve filtreler. |
| 4 | [mobil-ofis-04-uyarilar.png](mobil-ofis-04-uyarilar.png) | Uyarılar. |
| 5 | [mobil-ofis-05-belge-gorevleri.png](mobil-ofis-05-belge-gorevleri.png) | Belge görevleri. |

# Ekran listesi

"Pano" sütunu tuvaldeki artboard dosyasıdır: [`tuval/`](tuval/) altında `<ad>.dc.html`. "Hedef" sütunu kodda karşılık gelen yer. Öncelik numaraları README'deki sıraya göre.

## 1 · Tasarım sistemi

| Pano | İçerik |
|---|---|
| `Main` | Logo (açık, koyu, mavi zemin), uygulama ikonları, 4 temada renk token tablosu, tipografi, boşluk, köşe, aktarım |
| `Kristal` | Taç rozeti (5 durum), güven ölçeği, kristal yükleyici, ikon seti (18), boş durum, hareket ilkeleri |
| `Bilesenler` | Button, Badge ve taç, form, hesap kodu seçici, öneri kartı, toast, sekme ve filtre, komut paleti |

## 2 · Ofis web paneli (Next.js `frontend/`, `/t/[slug]`)

| Öncelik | Pano | Hedef | Önemli kararlar |
|---|---|---|---|
| 3 | `OfisNav` | `config/nav.ts` + kenar menü bileşeni | Gruplar: Bugün / Kayıtlar / Ofis. Sayaçlar menüde. Altta ajan durumu ve kullanıcı. |
| 3 | `OfisPano` | `/t/[slug]` (`office-home.tsx`) | "Dönem tacı": veri %, onay %, beyana hazır x/y. Selamlama satırında KDV son gün. KPI'lar tıklanır. İşi geride olan mükellefler. KDV tahmini. "Bugün olanlar" akışı. |
| 3 | `OfisKuyruk` | `/review` + `/review/studio` birleşir | Sol: en düşük güven önce. Orta: kaynak satır + "Neden bu kod?" (eşik çizgili güven çubuğu) + karşı taraf geçmişi. Eylemler A / E / K / R, Z geri al. Üstte "%95 üstü N öneriyi toplu onayla" şeridi. Düzelt paneli combobox; "bu karşı taraf için kural yap" onay kutusu. |
| 6 | `OfisMukellefler` | `/taxpayers` + engagement ayrıntısı | Liste ve ayrıntı yan yana. Ayrıntıda 6 aylık dönem şeridi (her ay bir taç). Sekmeler: özet, faturalar, fişler, banka, görevler, mizan hafızası, dilekçeler. Beyana hazırlık, KDV tahmini, açık belge görevleri. |
| 6 | `OfisBelgeler` | `/invoices`, `/invoices/[id]`, `/upload` | Sekmeler: Faturalar / Fişler / Banka / Yüklemeler. Satır başında taç rozeti. Yükleme işi ilerleme şeridi. Sağ çekmecede tutar şelalesi ve engelleyici uyarı. |
| 7 | `OfisMizan` | mizan hafızası paneli (#310) + `accountrules` | KPI: kuralla / geçmişten / bozan düzeltme. Kurallar tablosu (sıra, koşul, hesap, kapsam, isabet, durum). Hafıza dağılım çubukları. Kural editöründe "kaydetmeden önce" önizleme. |
| 6 | `OfisUyarilar` | `/warnings` | Seviye kartları (Engelleyici / Uyarı / Bilgi). Koda göre gruplu. Satır eylemleri: Gördüm / Faturayı aç / Çözüldü. |
| 6 | `OfisLuca` | `/journals` | Fiş listesi (Taslak / Onaylı / Aktarıldı). Sağ panel: ön kontrol, program seçimi (Luca / Excel / Zirve, "şablon doğrulanmadı" notu), çıktı ve dönem kilidi. Önceki çıktılar. |
| 7 | `OfisKurulum` | `/setup` | 6 adım ve sol adım listesi (Bekliyor / Tamamlandı / Atlandı). Mükellef listesinde satır içi düzeltme. |
| 7 | `OfisAyarlar` | `/settings`, `/agents`, `/members`, `/roles` | Alt menü. Yapay zekâ ve onay (eşik kaydırıcısı ve etki özeti, kural istisnası, stajyer onayı). Masaüstü ajanları tablosu. Görünüm: 4 tema + açık / koyu / sistem. |
| 9 | `OfisDuyarli` | bütün ofis ekranları | Tablet: 72 px ikon rayı + liste / ayrıntı. Telefon: alt sekme (Pano / Kuyruk / Mükellef / Uyarı / Daha), tablolar listeye dönüşür. |
| — | `TemaKarsilastirma` | görsel test referansı | Pano 4 temada. |

## 3 · Mükellef

| Öncelik | Pano | Hedef | Önemli kararlar |
|---|---|---|---|
| 5 | `MukellefMobil` · 1 Ana | `mobile/src/app/(app)/index` | Büyük "Fiş / Z raporu çek" kartı. "Müşavirinin istedikleri". Çevrimdışı kuyruk durumu. Alt sekme: Ana / İstekler / Geçmiş / Profil. |
| 5 | · 2 Fiş çek | `capture` | Fiş türü seçici (Gider / Akaryakıt / Z raporu). Kenar kılavuzları, "Fiş algılandı" etiketi, seri çekim. |
| 5 | · 3 İstek | `task/[uuid]` | Kim istedi, son gün, not. Ek önizleme. "Bu belge bende yok". |
| 5 | · 4 Bildirimler | **yeni ekran** | Okunmamış satır vurgusu, tür ikonları. |
| 5 | · 5 Geçmiş | `receipts/index` | Fiş / Fatura / İstek sekmeleri. Kuyruk etiketleri renkli. |
| 8 | `MukellefWeb` | `/m/[taxpayer]` | Üst menü, ofis isteği onay şeridi (KVKK), istek listesi, sürükle bırak alanı, son gönderilenler. |
| 8 | `MukellefTablet` | mobil, yatay | Ray + istek listesi + ayrıntı. |

## 4 · Mali müşavir mobil (`mobile-office/`)

| Öncelik | Pano | Hedef | Önemli kararlar |
|---|---|---|---|
| 4 | `MusavirMobil` · 1 Pano | `(app)/index` | Dönem tacı (yalnızca kendi mükellefleri). Onay kuyruğu birincil kart (tahmini süreyle). 3 sayaç. Bugün gelenler. |
| 4 | · 2 Onay | `review/index` | Kart yığını: sağa onay, sola red, yukarı düzelt. Alt düğmeler aynı eylemler (erişilebilirlik). İlerleme çubuğu. |
| 4 | · 3 Mükellefler | `clients/index` | Arama, satırda taç durumu. |
| 4 | · 4 Belge iste | `tasks/new` | Hazır belge çipleri, not, son gün (3 / 7 / 14 / tarih). |
| 4 | · 5 Bildirimler | **yeni ekran** | Tümü / Engel / Yükleme filtreleri. |
| 8 | `MusavirTablet` | yatay | Mükellefe göre gruplu liste, ortada belge görseli, sağda karar paneli. |

## 5 · Masaüstü ajanı (`desktop/`), öncelik 2, baştan

| Pano | İçerik |
|---|---|
| `AjanTepsi` | Tepsi ikonu 5 durumu (16 px tek renk: bağlı, gönderiyor, duraklatıldı, sorun, güncelleme). macOS menüsü (durum, 3 sayaç, son dosyalar, eylemler). Windows sorun menüsü (yeniden dene). Bildirim kuralları: başarıda sessiz; ağ hatası 30 dk sürerse tek bildirim; kalıcı hatalar gün sonu özeti. |
| `AjanPencere` | Kenar sekmeler: Klasörler / Geçmiş / Sorunlar / Muhasebe programı / Ayarlar. Klasör satırında mükellef, yol, son eşitleme, durum. Sorunlar geçici ve kalıcı olarak ayrılır, geçici olanlarda deneme çubuğu. Güncelleme kartı. |
| `AjanKurulum` | 3 adım: giriş (sunucu adresi gizli "gelişmiş"), ofis ve bilgisayar adı, ilk klasör (mükellef + klasör + bulunan dosya sayısı). |

## 6 · Ortak

| Pano | İçerik |
|---|---|
| `Giris` | Web: bölünmüş giriş (siyah marka paneli + form, QR ile giriş). Mobil: 6 haneli doğrulama kodu. Mükellef ilk açılış (üç kristal anlatımı + KVKK onayı). |
| `Durumlar` | Boş (başarı, ilk kullanım). Yükleme (iskelet + kristal). Hata (geçici, verinin güvende olduğu söylenir). Yetki 403. "Dönem tamam" kutlaması. Açık ve koyu yan yana. |

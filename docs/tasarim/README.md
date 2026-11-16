# HAKAN arayüz tasarımı · v1 teslim notu

Durum: **Onaylandı** (16 Kasım 2026). Bütün yüzeyler için uygulamaya hazır.

- Tasarım tuvali (Claude Design): https://claude.ai/artifact/LSEbRQkoL4HrJMZrYNg7zb. Tuval sahibine özeldir; erişim için ürün sahibinden paylaşım isteyin.
- Tuvalin kaynak dosyaları: [`tuval/`](tuval/). Her ekran bir `.dc.html` dosyası; ortak stiller [`tuval/hakan.css`](tuval/hakan.css). Tuvale erişimi olmayanlar bu dosyalardan ölçü, metin ve örnek veri okuyabilir.
- Token'lar: [`tokens/`](tokens/)
  - [`tokens.css`](tokens/tokens.css): web ve Tauri.
  - [`tailwind-theme.css`](tokens/tailwind-theme.css): Tailwind v4 / shadcn eşlemesi.
  - [`tokens.ts`](tokens/tokens.ts): Expo.
- Ekran listesi: [`ekranlar.md`](ekranlar.md). Bileşen listesi: [`bilesenler.md`](bilesenler.md).

## Marka ve ilkeler

Logo **M3 "Taç kristal"** (`tasarim-referans/logo-ornekleri.pdf`) eski D19 işaretinin (borç/alacak "H") yerine geçer. Renkler PDF'ten piksel ölçümüyle alındı.

| Rol | M3 (ana) | N3 (alternatif tema) |
|---|---|---|
| Açık kristal · mükellef | `#CFD8FF` | `#D4D3D8` |
| Orta kristal · beyanname | `#2448FF` | `#01FF11` |
| Koyu kristal · mali müşavir | `#102290` | `#0A0A0A` |
| Siyah / zemin | `#0A0A0A` / `#F3F3F5` | `#0A0A0A` / `#F3F3F5` |

1. **Kristal netliği:**
   - Az renk ve keskin hatlar.
   - Tutarlar her zaman `tabular-nums`.
   - Hesap kodu, VKN ve ETTN mono yazılır.
2. **Önce karar:** Her ekran bir sonraki işi öne çıkarır (onayla, düzelt, kural yap).
3. **Üç taraf, bir taç:** Durum dili logodan gelir.
   - Belge **taç rozeti**, aşamaya göre dolar:
     - Bekleniyor (boş)
     - Yüklendi / öneri hazır (sol kristal dolu)
     - Müşavir onayladı (sol ve sağ kristal dolu)
     - Luca'ya aktarıldı (üçü de dolu)
     - Sorun var (kırmızı)
   - AI güveni üç kademeli kristalle gösterilir:
     - Yüksek (≥ %90 ya da kural eşleşmesi)
     - Orta (%60–90)
     - Düşük (< %60)
     - Yüzde değeri yalnızca üzerine gelince ya da ayrıntıda görünür.
   - Yükleme animasyonu: üç kristal sırayla yükselir. 400 ms'den kısa işlemlerde gösterilmez; listeler iskelet kullanır.
   - Boş durumlar kesik çizgili kristallerle çizilir.

**Temalar:**
- Dört tema var: `m3-light`, `m3-dark`, `n3-light`, `n3-dark`.
- Varsayılan M3; kullanıcı Ayarlar → Görünüm'den seçer, "sistemi izle" varsayılan.
- N3'te yeşil yalnızca dolgu olarak kullanılır, üstündeki metin siyahtır. Beyaz zeminde yeşil metin gerekirse `brand-text` (`#067A12`) kullanılır.

**Erişilebilirlik:**
- Metin kontrastı en az 4.5:1.
- Durum renkleri yalnızca renkle ayrılmaz; her durumun ikonu ve etiketi vardır.
- Dokunma hedefleri en az 44 px.
- "Hareketi azalt" açıksa yalnızca opaklık geçişi kalır.

**Hareket:**

| Süre | Kullanım |
|---|---|
| 120 ms | hover, basma |
| 200 ms | panel, menü, toast |
| 320 ms | kaydırarak onay |

Mobil kaydırma yay ile çalışır (Reanimated `withSpring`, damping 18). Tek kutlama anı onaydır: taçtaki kristal dolar.

**Tipografi:**
- Display ve başlıklar: Inter Tight. Gövde: Inter. Kod ve sayı etiketleri: JetBrains Mono. Geist bırakılır.
- Web gövde 14/20. Mobil gövde 15/22 (Dynamic Type ile ölçeklenir). Tauri ajanı 13/18.

**Izgara:** 4 px. Köşe yarıçapları 6 / 10 / 14 / 22. "Kristal kesik köşe" (`clip-path` ile 14 px pah) yalnızca vurgulu kartlarda kullanılır: pano özeti ve onay kartı.

**Kırılımlar:**

| Genişlik | Yerleşim |
|---|---|
| ≥ 1280 | Tam kenar menü, çok sütun |
| 768–1279 | İkon rayı, iki panel (liste + ayrıntı) |
| < 768 | Alt sekme çubuğu, tek sütun; tablolar listeye dönüşür |

## Uygulama öncelik sırası

Koordinatör şeritleri bu sıraya göre dağıtır. Her adım kendi PR'ı ile gelir.

1. **Temel (her şeyi bloklar):**
   - Token dosyalarını yerleştir: web `frontend/src/styles/`, Expo `packages/mobile-core/src/ui/theme.ts`, Tauri `desktop/ui/style.css`.
   - Fontları yükle: Inter, Inter Tight, JetBrains Mono.
   - Logo varlıklarını (M3 SVG ve uygulama ikonları) `docs/brand/` ve uygulamalara koy.
   - `TacRozeti`, `GuvenOlcegi`, `KristalYukleyici` ortak bileşenlerini yaz: web, Expo ve Tauri.
2. **Masaüstü ajanı:** Tamamen yeni arayüz. Kullanıcının en çok şikâyet ettiği yüzey bu. Tepsi menüsü, ana pencere (5 sekme), kurulum (3 adım), bildirim kuralları.
3. **Ofis web, çekirdek akış:**
   - Kenar menü ve üst çubuk. Global dönem seçici bu çubukta.
   - İnceleme kuyruğu. İnceleme stüdyosu ile kuyruk tek ekranda birleşir.
   - Genel bakış (dönem tacı).
4. **Mali müşavir mobil:** Pano, kaydırarak onay, bildirim listesi (yeni), belge iste.
5. **Mükellef mobil:** Ana sayfa (büyük çekim düğmesi), fiş çek, istekler, geçmiş, bildirim listesi (yeni), alt sekme çubuğu.
6. **Ofis web, kayıtlar:** Mükellefler (liste + ayrıntı + dönem şeridi), faturalar ve fişler (ayrıntı çekmecesi + tutar şelalesi), uyarılar (koda göre gruplu), yevmiye ve Luca çıktısı.
7. **Ofis web, yapılandırma:** Mizan hafızası ve kurallar, kurulum sihirbazı, ayarlar (AI ve onay eşiği, ajanlar, görünüm).
8. **Mükellef web portalı ve tablet yerleşimleri.**
9. **Ortak:** Giriş ve onboarding ekranları; boş, yükleme ve hata durum bileşenleri; koyu mod denetimi (dört temada görsel test).

## Ürün kodunda karşılığı olmayan, tasarımda yeni olanlar

Uygulayan şerit backend ucunu doğrulamalı; eksikse ayrı iş açılır.

- Mobil **bildirim listesi**, iki uygulamada da (şu an yalnızca push var). Backend'de bildirim listeleme ucu gerekir.
- **Global dönem seçici** (üst çubuk). Seçilen dönem bütün listeleri filtreler.
- Uyarılarda **koda göre gruplama** ve "Tümünü gördüm".
- Kural editöründe **"kaydetmeden önce" önizlemesi**: son 6 ayda kaç belgeye uyduğu ve kaçının farklı kodla onaylandığı.
- Ayarlarda **otomatik onay eşiği** kaydırıcısı ve geçmişe dönük etki özeti. Backend eşiği şu an sabit 0.95 (`review/usecase/policy.go`).
- Mükellef dönem şeridi: ay başına bir taç.
- Banka hareketleri sekmesi: backend var, arayüz yok.

Not: Mizan hafızası paneli bu tasarım sürerken #310 ile eklendi. Mevcut ekranın tuvaldeki "Mizan hafızası ve kurallar" yerleşimine taşınması yeterli.

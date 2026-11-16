# Bileşen listesi

Web bileşenleri `frontend/src/components/ui` (shadcn tabanlı). Expo bileşenleri `packages/mobile-core/src/ui` (iki uygulama ortak). Tauri ajanı web CSS'ini paylaşır: vanilla, token sınıfları. Aynı ad ve aynı prop'lar kullanılır.

## Yeni, HAKAN'a özgü (öncelik 1)

| Bileşen | Prop'lar | Not |
|---|---|---|
| `TacRozeti` | `durum: 'bekleniyor'│'yuklendi'│'onaylandi'│'aktarildi'│'sorun'`, `boyut: 16│30│72` | Üç kristal (yanlar 11/16, orta 16/16 oranında). Dolgular `--c-light / --c-dark / --c-mid`. Sorun durumunda `--danger`. Her zaman bir metin etiketiyle birlikte ya da `aria-label` ile. |
| `GuvenOlcegi` | `deger: 0..1`, `kaynak?: 'kural'│'gecmis'│'model'│'girdi'` | 3 kademe: < .6 düşük (`--warn`), < .9 orta (`--brand-text`), aksi yüksek (`--ok`); kural eşleşmesi her zaman yüksek. Yüzde ipucu olarak gösterilir. |
| `KristalYukleyici` | `boyut` | 3 kristal sırayla yükselir: 1.4 sn döngü, 180 ms gecikme. Reduced motion açıksa opaklık geçişi. |
| `BosDurum` | `baslik`, `aciklama`, `eylem?` | Kesik çizgili kristaller + tek eylem. |
| `HataDurumu` | `baslik`, `verilerGuvende: boolean`, `kod?`, `onRetry` | Teknik kod "Ayrıntı" bağlantısının içinde. |
| `DonemTaci` | `veri%`, `onay%`, `hazir`, `toplam` | Pano kahraman kartı, kesik köşe. |
| `DonemSeridi` | `donemler[]` (her biri taç durumu + etiket) | Mükellef ayrıntısı. |
| `OneriKarti` | belge özeti, önerilen kod, güven, neden, eylemler | Web kuyruğu, müşavir mobil kartı, tablet karar paneli aynı veri modelini kullanır. |
| `HesapKoduSecici` | `value`, `mukellefId`, `karsiTaraf?` | Combobox. Hafızada geçen kodlar üstte, "N×" rozetiyle. Kod mono yazılır. |
| `TutarSelalesi` | `matrah`, `vergiler[]`, `tevkifat`, `faturadaki` | Fark varsa kırmızı ve açıklamalı. |
| `KaydirmaKarti` (Expo) | `onApprove`, `onReject`, `onEdit` | Reanimated + Gesture Handler. Eşik %35 genişlik. Alt düğmeler zorunlu. |
| `TepsiIkonu` (Tauri) | `durum` | 16 px template image (macOS), Windows'ta ICO setleri. |
| `MarkaLogosu` | `varyant: 'isaret'│'yazi'│'tam'`, `zemin` | M3 SVG. Kristal yüzleri token'dan gelir (N3'te otomatik değişir). |

## Mevcut shadcn bileşenlerinin yeni görünümü

| Bileşen | Değişiklik |
|---|---|
| Button | Yükseklik 30 / 36 / 44. Varyantlar: `primary` (brand), `dark`, `outline`, `ghost`, `ok`, `danger-outline`. İçinde kısayol için `Kbd` yuvası. |
| Badge (pill) | 22 px, yuvarlak. Varyantlar: nötr, brand, ok, warn, danger. Taç rozeti alabilir. |
| Input / Label | 40 px (mobilde 46). Odakta `--focus` halkası 3 px tint. Hata metni altta. |
| Table | Başlıklar mono 11 px büyük harf. Seçili satırda `--brand-tint` ve 3 px sol iç gölge. < 768 px'de listeye dönüşür. |
| Tabs / Segmented | `surface-3` yuvası, seçili segment beyaz ve gölgeli. |
| Toast (sonner) | Koyu (metin rengi zemin). Toplu işlemlerde 6 sn "Geri al". |
| Command (cmdk) | ⌘K: mükellef, belge, ekran araması. |
| Sheet / Drawer | Sağ ayrıntı çekmecesi 380–460 px. Mobilde alttan sayfa (köşe 22). |
| Switch / Checkbox | Brand dolgu, `on-brand` başparmak. |

## Kenar menü ve kabuk

- `OfisKabugu`: `≥1280` 240 px menü; `768–1279` 72 px ikon rayı; `<768` alt sekme çubuğu.
- Üst çubuk: arama (⌘K), **global dönem seçici**, bildirimler, birincil eylem (UBL yükle).
- `MukellefPortalKabugu`: üst yatay menü, şirket seçici.

## İkonlar

Lucide temeldir (`lucide-react`, `lucide-react-native`), çizgi kalınlığı 1.75. HAKAN'a özgü ikonlar (pano, mizan, Luca çıktısı, kural yap, klasör izle) `Kristal` panosundaki çizimlerden SVG olarak çıkarılır: `packages/ui/icons`.

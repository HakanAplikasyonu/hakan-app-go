# Marka dosyaları (D19)

Kaynak kanvas: https://claude.ai/artifact/X3LAbL8LTy645XLYtAL16N. "Hakan" geçici ad; işaret ad değişse de kalır.

## İşaret

| Dosya | Kullanım |
|---|---|
| `logo-mark.svg` | Ana işaret, açık zemin (koyu karo, beyaz sütunlar, `#5b8cff` onay) |
| `icons/logo-mark-dark.svg` | Koyu zemin (beyaz karo, koyu sütunlar, `#1447e6` onay) |

## Uygulama ikonları

| Dosya | Hedef |
|---|---|
| `icons/app-icon-ios.svg` → `png/ios-1024.png` | iOS App Store. Köşesiz; maskeyi iOS uygular. |
| `icons/app-icon-rounded.svg` → `png/android-play-512.png`, `png/apple-touch-180.png`, `png/pwa-192.png` | Play Store, apple-touch, PWA |
| `icons/android-adaptive.svg` | Android adaptive önizleme (daire maske) |
| `icons/android-adaptive-foreground.svg` → `png/android-adaptive-foreground-432.png` | Adaptive ön plan, 108dp kanvas, 66dp güvenli alan. Arka plan rengi `#0f172b`. |
| `icons/tauri.svg` → `png/tauri-1024.png` | Masaüstü. `cargo tauri icon docs/brand/icons/png/tauri-1024.png` ile tüm boyutlar üretilir. |
| `icons/favicon-48.svg`, `favicon-32.svg`, `favicon-16.svg` → `favicon.ico` | Favicon. 16 px'te onay çizgisi okunmadığı için çıkarıldı (kanvastaki gibi). |

PNG'ler SVG'lerden Chromium ile üretildi. SVG değişirse PNG'ler de yeniden üretilmeli.

Frontend: `frontend/src/app/icon.svg` = `favicon-32.svg`, `frontend/src/app/favicon.ico` = `icons/favicon.ico`, `apple-icon.tsx` aynı işareti çizer.

## Renk tokenları

`frontend/src/styles/theme.css`. Boilerplate'in slate tokenları korunur, üstüne:

| Token | Açık | Koyu | Tailwind |
|---|---|---|---|
| `--brand` | `#1447e6` | `#5b8cff` | `bg-brand`, `text-brand` |
| `--brand-check` | `#5b8cff` | `#1447e6` | logo onay çizgisi |
| `--status-approved` (onaylandı) | `#009689` | `#00bba7` | `bg-status-approved` |
| `--status-review` (inceleme) | `#ffb900` | `#ffb900` | `bg-status-review` |
| `--status-warning` (uyarı) | `#f54900` | `#ff6900` | `bg-status-warning` |
| `--status-blocking` (engelleyici) | `#e7000b` | `#fb2c36` | `bg-status-blocking` |

Her durum tokenının `-foreground` eşi var (ör. `text-status-review-foreground`). İnceleme sarısının üstüne koyu yazı gelir.

Koyu temada marka ve durum renkleri bir ton açıldı; `#1447e6` koyu zeminde metin olarak okunmuyor.

## Yazı

Geist (metin) ve Geist Mono. Tutar, hesap kodu, VKN ve ETTN her yerde `figure` sınıfıyla yazılır: Geist Mono + `tabular-nums`.

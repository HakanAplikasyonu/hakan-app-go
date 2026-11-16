# Marka dosyaları (arayüz v1, M3 "Taç kristal")

Kaynak: onaylı tasarım tuvali, `docs/tasarim/README.md` ve `docs/tasarim/tuval/Main.dc.html`. M3 işareti eski D19 işaretinin (borç/alacak "H") yerine geçer (#324). "Hakan" geçici ad; işaret ad değişse de kalır.

## İşaret

Üç kristal: sol **açık** (mükellef), orta **mavi** (beyanname / Luca), sağ **lacivert** (mali müşavir). `viewBox="0 0 181 198"`.

| Rol | M3 | N3 (alternatif tema) |
|---|---|---|
| Açık kristal | `#CFD8FF` | `#D4D3D8` |
| Orta kristal | `#2448FF` | `#01FF11` |
| Koyu kristal | `#102290` | `#0A0A0A` (koyu temada `#F3F3F5`) |

| Dosya | Kullanım |
|---|---|
| `logo-mark.svg` | Ana işaret. Açık (`#F3F3F5`) ve siyah (`#0A0A0A`) zeminde aynı renkler. |
| `icons/logo-mark-dark.svg` | Siyah zemin; `logo-mark.svg` ile aynı (tuvalde ayrı renk yok). |
| `icons/logo-mark-brand.svg` | Mavi (`#2448FF`) zemin: orta kristal beyaz. |

Uygulamalarda işaret token'dan boyanır (`--c-light / --c-mid / --c-dark`), N3 seçilince kendiliğinden değişir: web `AppMark` / `MarkaLogosu`, Expo `MarkaLogosu` (`@hakan/mobile-core/hakan`).

## Uygulama ikonları

Mükellef uygulaması ve web **siyah** karo, müşavir (ofis) uygulaması ve masaüstü ajanı **mavi** karo (orta kristal beyaz); ana ekranda karışmaz. Karoda işaret yüksekliği ≈ %58 (tuval: 64 px karoda 37 px).

| Dosya | Hedef |
|---|---|
| `icons/app-icon-ios.svg` → `png/ios-1024.png` | iOS, mükellef (`mobile/assets/icon.png`). Köşesiz; maskeyi iOS uygular. |
| `icons/app-icon-ios-office.svg` → `png/ios-office-1024.png` | iOS, ofis (`mobile-office/assets/icon.png`). |
| `icons/app-icon-rounded.svg` → `png/android-play-512.png`, `png/apple-touch-180.png`, `png/pwa-192.png` | Play Store, apple-touch, PWA (mükellef ve web). |
| `icons/app-icon-rounded-office.svg` → `png/android-play-office-512.png`, `png/splash-office-512.png`, `png/favicon-office-48.png` | Ofis: Play Store, açılış ekranı, Expo web favicon. |
| `icons/android-adaptive.svg` | Android adaptive önizleme (daire maske). |
| `icons/android-adaptive-foreground.svg` → `png/android-adaptive-foreground-432.png` | Adaptive ön plan (mükellef), 108dp kanvas. Arka plan `#0A0A0A`. |
| `icons/android-adaptive-foreground-office.svg` → `png/android-adaptive-foreground-office-432.png` | Adaptive ön plan (ofis). Arka plan `#2448FF`. |
| `icons/splash.svg` → `png/splash-512.png` | Mükellef açılış ekranı (zemin `#F3F3F5` / koyu `#0A0A0A`). |
| `icons/tauri.svg` → `png/tauri-1024.png` | Masaüstü ajanı (mavi karo). `cd desktop && npm run icons` tüm boyutları üretir. |
| `icons/favicon-48.svg`, `favicon-32.svg`, `favicon-16.svg` → `favicon.ico` | Web favicon (siyah karo). |

PNG'ler SVG'lerden Chromium ile üretildi (`deviceScaleFactor: 1`, saydam zemin). SVG değişirse PNG'ler de yeniden üretilmeli.

Frontend: `frontend/src/app/icon.svg` = `favicon-32.svg`, `frontend/src/app/favicon.ico` = `icons/favicon.ico`, `apple-icon.tsx` aynı işareti siyah karoda çizer. Tauri: `desktop/ui/logo.svg` = `logo-mark.svg`.

## Renk ve token'lar

Tek kaynak `docs/tasarim/tokens/`. Uygulamalardaki kopyalar:

| Yüzey | Dosya |
|---|---|
| Web (Next.js, Tailwind v4) | `frontend/src/styles/tokens.css` + `theme.css` (shadcn eşlemesi, eski adlar için takma adlar) |
| Expo (iki uygulama) | `packages/mobile-core/src/ui/tokens.ts` (+ `theme.ts`, `palette.ts` eski adlar) |
| Tauri ajanı | `desktop/ui/tokens.css` (+ `style.css` eski adlar) |

Dört tema: `m3-light`, `m3-dark`, `n3-light`, `n3-dark` (`<html data-theme>`). Varsayılan M3, "sistemi izle".

## Yazı

Başlık Inter Tight, gövde Inter, kod ve sayı etiketi JetBrains Mono. Geist bırakıldı. Tutarlar `tabular-nums` (web `num`), hesap kodu / VKN / ETTN mono (web `figure`). Web `next/font/google`, Expo `@expo-google-fonts/*`, Tauri çevrimdışı `desktop/ui/fonts/` (OFL-1.1).

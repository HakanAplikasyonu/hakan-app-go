# Native bileşen denetimi (#519)

Tarih: 19 Kasım 2026. Kapsam: `frontend/src` altında, `src/components/ui` dışında kalan `.tsx` dosyaları.
`type="month"` satırlarında `Input type="month"` de sayıldı (`engagement-payroll-page`, `journals-page`, `taxpayer-invoices-page`).
Bu belge yalnızca araştırmadır; değişiklikler #520'de, özellik alanı başına bir PR ile yapılır.

## Yöntem

```bash
cd frontend
# ham form öğeleri ve tablolar
rg -n --glob '*.tsx' -g '!src/components/ui/**' \
  '<(button|input|select|textarea|table|label|details|summary|dialog|progress|meter|fieldset)(\s|>|$)' src
# tarayıcı diyalogları
rg -n -g '!src/components/ui/**' '\b(window\.)?(alert|confirm|prompt)\(' src
# native tarih/renk/dosya girişleri
rg -n --glob '*.tsx' 'type="(color|checkbox|radio|file|month|time|date)"' src
```

`confirm(...)` / `prompt(...)` çağrılarının çoğu `useDialogs()` (`src/providers/dialog-provider.tsx`) üzerinden gelir; bunlar zaten shadcn `Dialog` kullanır ve native değildir. Satır numaraları `origin/main` (db4975d) içindir.

## Özet

| Öğe | Adet | Önerilen karşılık |
|---|---:|---|
| `<button>` | 38 | `Button` (`variant="ghost"`/`"link"`, gerekirse `asChild`) |
| `<label>` | 53 | `Label` ya da `Field`/`FieldLabel` (`ui/field.tsx`) |
| `<input>` | 28 | Metin → `Input`/`InputGroup`; checkbox → `Checkbox`; radio → `RadioGroup`; dosya → istisna (aşağıda) |
| `type="month"` (`Input` üzerinde dâhil) | 4 | `MonthPicker` (eklenecek, `date-picker` ailesi) |
| `<select>` | 8 | `Select`; yoğun tablo filtrelerinde `NativeSelect` (eklenecek) |
| `<textarea>` | 1 | `Textarea` |
| `<table>` | 13 (3 istisna) | `Table` (eklenecek) |
| `<fieldset>` | 4 | `FieldSet`/`FieldLegend` (`ui/field.tsx`) ya da `RadioGroup` |
| `<details>/<summary>` | 1 | `Collapsible` |
| `window.prompt()` | 1 | `useDialogs().prompt` |
| `alert()` | 2 | `toast` (sonner) — editör eklentisi, istisna adayı |
| `title=` ipucu | ~234 | İkon düğmelerinde `Tooltip` + `aria-label`; ayrıntı aşağıda |

## Dosya dosya bulgular

Sütunlar: dosya:satır · native öğe · önerilen shadcn karşılığı · istisna mı.

### Ofis — banka

| Yer | Öğe | Öneri | İstisna |
|---|---|---|---|
| `features/office/components/bank-statement-upload.tsx:159` | `input` (dosya) | Gizli dosya girişi, tetikleyici `Button` | Evet (dosya) |
| `features/office/components/bank-statement-upload.tsx:201` | `table` | `Table` | Hayır |
| `features/office/components/engagement-bank-page.tsx:295` | `button` | `Button variant="ghost"` | Hayır |
| `features/office/components/engagement-bank-page.tsx:386` | `button` | `Button variant="ghost"` | Hayır |
| `features/office/components/bank-posting-panel.tsx:312` | `button` | `Button` | Hayır |
| `features/office/components/luca-bank-export-panel.tsx:198` | `label` | `Label` | Hayır |
| `features/office/components/luca-bank-export-panel.tsx:208` | `label` | `Label` | Hayır |
| `features/office/components/luca-bank-export-panel.tsx:302` | `table` | `Table` | Hayır |

### Ofis — beyanname, bordro, Z raporu

| Yer | Öğe | Öneri | İstisna |
|---|---|---|---|
| `features/office/components/engagement-tax-returns-page.tsx:255` | `button` | `Button` | Hayır |
| `features/office/components/engagement-tax-returns-page.tsx:551` | `label` | `Label` | Hayır |
| `features/office/components/engagement-tax-returns-page.tsx:556` | `label` | `Field`/`FieldLabel` | Hayır |
| `features/office/components/engagement-tax-returns-page.tsx:560` | `input` | `Input` | Hayır |
| `features/office/components/engagement-tax-returns-page.tsx:654` | `label` | `Label` | Hayır |
| `features/office/components/engagement-tax-returns-page.tsx:655` | `input checkbox` | `Checkbox` | Hayır |
| `features/office/components/engagement-tax-returns-page.tsx:706` | `table` | `Table` | Hayır |
| `features/office/components/engagement-tax-returns-page.tsx:813` | `label` | `Field`/`FieldLabel` | Hayır |
| `features/office/components/engagement-tax-returns-page.tsx:817` | `input` | `Input` | Hayır |
| `features/office/components/engagement-payroll-page.tsx:576` | `label` | `Field`/`FieldLabel` | Hayır |
| `features/office/components/engagement-payroll-page.tsx:580` | `Input type="file"` | Zaten shadcn `Input` | — |
| `features/office/components/engagement-payroll-page.tsx:588` | `label` | `Field`/`FieldLabel` | Hayır |
| `features/office/components/engagement-payroll-page.tsx:591` | `Input type="month"` | `MonthPicker` | Hayır |
| `features/office/components/engagement-payroll-page.tsx:1036` | `label` | `Field`/`FieldLabel` | Hayır |
| `features/office/components/engagement-payroll-page.tsx:1040` | `Input type="file"` | Zaten shadcn `Input` | — |
| `features/office/components/engagement-payroll-page.tsx:1045` | `label` | `Field`/`FieldLabel` | Hayır |
| `features/office/components/engagement-payroll-page.tsx:1048` | `Input type="month"` | `MonthPicker` | Hayır |
| `features/office/components/engagement-payroll-page.tsx:1054` | `label` | `Label` | Hayır |
| `features/office/components/engagement-payroll-page.tsx:1055` | `input checkbox` | `Checkbox` | Hayır |
| `features/office/components/engagement-zreports-page.tsx:468` | `button` | `Button` | Hayır |
| `features/office/components/engagement-zreports-page.tsx:486` | `label` | `Label` | Hayır |
| `features/office/components/engagement-zreports-page.tsx:490` | `select` | `Select` | Hayır |

### Ofis — mutabakat

| Yer | Öğe | Öneri | İstisna |
|---|---|---|---|
| `features/office/components/engagement-reconciliation-page.tsx:170` | `button` | `Button` | Hayır |
| `features/office/components/engagement-reconciliation-page.tsx:539` | `label` | `Label` | Hayır |
| `features/office/components/engagement-reconciliation-page.tsx:540` | `input checkbox` | `Checkbox` | Hayır |
| `features/office/components/engagement-reconciliation-page.tsx:641` | `table` | `Table` | Hayır |
| `features/office/components/engagement-reconciliation-page.tsx:734` | `table` | `Table` | Hayır |
| `features/office/components/reconciliation-ui.tsx:114` | `label` | `Field`/`FieldLabel` | Hayır |
| `features/office/components/reconciliation-cari.tsx:383` | `label` | `Label` | Hayır |
| `features/office/components/reconciliation-cari.tsx:384` | `input checkbox` | `Checkbox` | Hayır |
| `features/office/components/reconciliation-cari.tsx:520` | `label` | `Field`/`FieldLabel` | Hayır |
| `features/office/components/reconciliation-cari.tsx:531` | `label` | `Field`/`FieldLabel` | Hayır |
| `features/office/components/reconciliation-cari.tsx:540` | `label` | `Label` | Hayır |
| `features/office/components/reconciliation-cari.tsx:541` | `input checkbox` | `Checkbox` | Hayır |
| `features/office/components/reconciliation-cari.tsx:714` | `label` | `Label` | Hayır |
| `features/office/components/reconciliation-cari.tsx:718` | `select` | `Select` | Hayır |
| `features/office/components/reconciliation-cari.tsx:737` | `input` | `Input` | Hayır |
| `features/office/components/reconciliation-cari.tsx:1085` | `fieldset` | `RadioGroup` | Hayır |
| `features/office/components/reconciliation-cari.tsx:1090` | `label` | `RadioGroupItem` + `Label` | Hayır |
| `features/office/components/reconciliation-cari.tsx:1091` | `input radio` | `RadioGroupItem` | Hayır |
| `features/office/components/reconciliation-cari.tsx:1102` | `label` | `Field`/`FieldLabel` | Hayır |
| `features/office/components/reconciliation-cari.tsx:1113` | `label` | `Field`/`FieldLabel` | Hayır |
| `features/office/components/reconciliation-cari.tsx:1119` | `label` | `Field`/`FieldLabel` | Hayır |
| `features/reconciliation/components/letter-response-page.tsx:146` | `fieldset` | `RadioGroup` | Hayır |
| `features/reconciliation/components/letter-response-page.tsx:151` | `label` | `RadioGroupItem` + `Label` | Hayır |
| `features/reconciliation/components/letter-response-page.tsx:155` | `input radio` | `RadioGroupItem` | Hayır |
| `features/reconciliation/components/letter-response-page.tsx:166` | `label` | `Field`/`FieldLabel` | Hayır |
| `features/reconciliation/components/letter-response-page.tsx:180` | `label` | `Field`/`FieldLabel` | Hayır |
| `features/reconciliation/components/letter-response-page.tsx:188` | `label` | `Field`/`FieldLabel` | Hayır |

### Ofis — masraf, fiş, yevmiye

| Yer | Öğe | Öneri | İstisna |
|---|---|---|---|
| `features/office/components/engagement-expenses-page.tsx:77` | `input` | `Input` (`size` küçük) | Hayır |
| `features/office/components/engagement-expenses-page.tsx:111` | `select` | `NativeSelect` (satır içi, tablo) | Hayır |
| `features/office/components/engagement-expenses-page.tsx:483` | `button` | `Button` | Hayır |
| `features/office/components/engagement-expenses-page.tsx:501` | `label` | `Label` | Hayır |
| `features/office/components/engagement-expenses-page.tsx:505` | `select` | `Select` | Hayır |
| `features/office/components/expense-categories-dialog.tsx:209,222,232,245` | `label` ×4 | `Field`/`FieldLabel` | Hayır |
| `features/office/components/expense-categories-dialog.tsx:254` | `label` | `Label` | Hayır |
| `features/office/components/expense-categories-dialog.tsx:255` | `input checkbox` | `Checkbox` | Hayır |
| `features/office/components/expense-categories-dialog.tsx:268` | `label` | `Label` | Hayır |
| `features/office/components/expense-categories-dialog.tsx:272` | `select` | `Select` | Hayır |
| `features/office/components/receipts-ui.tsx:64` | `input` | `Input` | Hayır |
| `features/office/components/journals-page.tsx:263` | `label` | `Label` | Hayır |
| `features/office/components/journals-page.tsx:691` | `window.prompt()` | `useDialogs().prompt` | Hayır |
| `features/office/components/journals-page.tsx:760` | `label` | `Label` | Hayır |
| `features/office/components/journal-double-booking.tsx:84,127,217` | `label` ×3 | `Label` | Hayır |
| `features/office/components/rule-editor.tsx:162,221` | `fieldset` ×2 | `FieldSet`/`FieldLegend` | Hayır |
| `features/office/components/rule-editor.tsx:195` | `select` | `Select` | Hayır |
| `features/office/components/account-memory-page.tsx:143` | `label` | `Label` | Hayır |
| `features/office/components/account-memory-page.tsx:145` | `input checkbox` | `Checkbox` | Hayır |

### Ofis — genel ekranlar ve kabuk

| Yer | Öğe | Öneri | İstisna |
|---|---|---|---|
| `features/office/components/command-center.tsx:141,250` | `button` ×2 | `Button` | Hayır |
| `features/office/components/command-center.tsx:227` | `label` | `Label` | Hayır |
| `features/office/components/command-center.tsx:228` | `input checkbox` | `Checkbox` | Hayır |
| `features/office/components/command-center.tsx:236–238` | `label`+`input` (arama) | `InputGroup` | Hayır |
| `features/office/components/review-workspace.tsx:1048` | `label` | `Label` | Hayır |
| `features/office/components/review-workspace.tsx:1058,1388` | `button` ×2 | `Button` | Hayır |
| `features/office/components/warnings-page.tsx:688,745` | `button` ×2 | `Button` | Hayır |
| `features/office/components/warning-actions.tsx:53` | `label` | `Label` | Hayır |
| `features/office/components/warning-actions.tsx:56` | `textarea` | `Textarea` | Hayır |
| `features/office/components/upload-page.tsx:295` | `label` (bırakma alanı) | `Label` ya da `Card` + `Button` | Hayır |
| `features/office/components/upload-page.tsx:316` | `input type="file"` | Gizli dosya girişi | Evet (dosya) |
| `features/office/components/taxpayers-page.tsx:104` | `button` | `Button` | Hayır |
| `features/office/components/taxpayers-page.tsx:240` | `input` | `Input` | Hayır |
| `features/office/components/office-ui.tsx:158` | `label` | `Label` | Hayır |
| `features/office/components/office-ui.tsx:160` | `select` (ortak filtre) | `Select` | Hayır |
| `features/office/components/related-records.tsx:119` | `table` | `Table` | Hayır |
| `features/office/components/change-requests-panel.tsx:163` | `table` | `Table` | Hayır |
| `features/office/components/engagement-layout.tsx:246` | `button` | `Button` | Hayır |
| `features/office/components/ai-approval-settings.tsx:100` | `label` | `Label` | Hayır |
| `features/office/components/invoice-preview.tsx:71,72` | `table` (HTML dizesi) | — | Evet (yazdırma/iframe HTML'i) |
| `features/office/shell/office-tabbar.tsx:92` | `button` | `Button` | Hayır |
| `features/office-setup/components/office-setup-page.tsx:88` | `button` | `Button` | Hayır |

### Mükellef paneli

| Yer | Öğe | Öneri | İstisna |
|---|---|---|---|
| `features/taxpayer/components/taxpayer-shell.tsx:206` | `button` | `Button` | Hayır |
| `features/taxpayer/components/taxpayer-requests-page.tsx:48` | `button` | `Button` | Hayır |
| `features/taxpayer/components/taxpayer-request-row.tsx:164` | `input type="file"` | Gizli dosya girişi | Evet (dosya) |
| `features/taxpayer/components/taxpayer-uploads-page.tsx:66` | `input type="file"` | Gizli dosya girişi | Evet (dosya) |
| `features/taxpayer/components/taxpayer-home.tsx:229` | `input type="file"` | Gizli dosya girişi | Evet (dosya) |

### Raporlar, yardımcı ekranlar

| Yer | Öğe | Öneri | İstisna |
|---|---|---|---|
| `features/reports/components/report-view.tsx:487–488` | `details`/`summary` | `Collapsible` | Hayır |
| `features/reports/components/report-view.tsx:492` | `table` | `Table` | Hayır |
| `features/reports/components/report-range-picker.tsx:66` | `button` | `Button` | Hayır |
| `features/copilot/components/command-bar.tsx:38` | `label` | `Label` | Hayır |
| `features/copilot/components/command-bar.tsx:67` | `button` | `Button` | Hayır |
| `features/copilot/components/copilot-page.tsx:256` | `label` | `Label` | Hayır |
| `features/copilot/components/plan-dialog.tsx:211` | `label` | `Label` | Hayır |
| `features/notifications/components/notification-inbox.tsx:181` | `button` | `Button` | Hayır |
| `features/search-engine/components/command-palette.tsx:73` | `button` (⌘K tetikleyici) | `Button variant="outline"` | Hayır |
| `features/settings/components/settings-sub-sidebar.tsx:204` | `button` | `Button` ya da `SidebarMenuButton` | Hayır |
| `features/storage/components/storage-file-list.tsx:72` | `button` | `Button` | Hayır |
| `features/storage/components/storage-file-grid.tsx:78` | `button` | `Button` | Hayır |
| `features/storage/components/storage-sidebar.tsx:46` | `button` | `Button` | Hayır |
| `features/storage/components/storage-explorer.tsx:452` | `input type="file"` | Gizli dosya girişi | Evet (dosya) |
| `features/storage/components/storage-upload-zone.tsx:60` | `input type="file"` | Gizli dosya girişi | Evet (dosya) |
| `features/tenant/components/tenant-roles-page.tsx:98` | `label` | `Label` | Hayır |
| `features/logs/components/purge-rule-form-drawer.tsx:61` | `label` | `Label` + `Checkbox` | Hayır |
| `features/legal/components/legal-markdown.tsx:292` | `table` | — | Evet (markdown çıktısı) |

### Ortak bileşenler

| Yer | Öğe | Öneri | İstisna |
|---|---|---|---|
| `components/tables/table-view.tsx:148` | `table` | `Table` | Hayır |
| `components/tables/editable-grid.tsx:779` | `table` | `Table` | Hayır |
| `components/tables/editable-cell.tsx:309` | `select` | — | Evet (yoğun ızgara hücresi, aşağıda) |
| `components/tables/editable-cell.tsx:327` | `input` | — | Evet (yoğun ızgara hücresi) |
| `components/tables/reorder-column.tsx:65` | `button` | `Button size="icon"` | Hayır |
| `components/forms/form-layout.tsx:113,151` | `button` ×2 | `Button` | Hayır |
| `components/upload/file-uploader.tsx:115` | `label` (bırakma alanı) | `Label` | Hayır |
| `components/upload/file-uploader.tsx:135` | `input type="file"` | Gizli dosya girişi | Evet (dosya) |
| `components/hakan/gorunum-ayari.tsx:102,149` | `button` ×2 | `ToggleGroup` | Hayır |
| `components/hakan/durumlar.tsx:109` | `button` | `Button` | Hayır |
| `components/branding/logo-control.tsx:102` | `input type="file"` | Gizli dosya girişi | Evet (dosya) |
| `components/realtime/realtime-debug-panel.tsx:148,193,202` | `button` ×3 | — | Evet (yalnızca geliştirme paneli) |
| `components/editor/**` (`color-picker.tsx:230`, `code-button.tsx:57`, `context-menu-plugin.tsx:74,102` `alert()`) | çeşitli | — | Evet (dışarıdan alınmış Lexical/shadcn-editor kodu) |
| `app/global-error.tsx:33` | `button` | — | Evet (kök layout çöktüğünde) |

## Bilinçli istisnalar

1. **`app/global-error.tsx`**: Kök layout çöktüğünde tema, sağlayıcılar ve bileşen paketi yüklenemeyebilir; sayfa kendi `<html>`'ini satır içi stille çizer.
2. **`features/legal/components/legal-markdown.tsx`**: Markdown'dan üretilen tablo; içerik belge, arayüz değil. (İleride `Table` bileşeniyle eşlenebilir ama zorunlu değil.)
3. **`features/office/components/invoice-preview.tsx`**: Yazdırma ve iframe için üretilen HTML dizesi; React bileşeni kullanılamaz.
4. **`components/editor/**`**: Lexical/shadcn-editor'dan alınmış kod; ESLint'te zaten ayrı kurallarla yönetiliyor. Güncellenebilirliği korumak için dokunulmaz. `alert()` çağrıları kopyalanmışsa `toast.error` ile değiştirilebilir, bu tek küçük düzeltme önerilir.
5. **`components/tables/editable-cell.tsx`**: Binlerce hücrelik ızgarada her hücreye Radix `Select`/`Popover` bağlamak klavye gezinmesini ve performansı bozar. Ham `input`/`select` kalır; ama stil `Input`/`NativeSelect` sınıflarıyla (`cn(inputVariants)`) hizalanır.
6. **Gizli `input type="file"`**: Dosya seçici tarayıcının kendi penceresidir; shadcn'de karşılığı yok. Görünen kısım `Button`/bırakma alanıdır. Kural: giriş `className="sr-only"` ya da `hidden` olmalı ve bir `Button`/`Label` ile tetiklenmeli. Görünür dosya girişi gerekiyorsa `Input type="file"` kullanılır.
7. **`components/realtime/realtime-debug-panel.tsx`**: Yalnızca geliştirme ortamında görünür.

## `title=` ipuçları

`ui` dışında ~234 `title=` kullanımı var. Üçe ayrılır:

- **İkon düğmeleri** (`Button size="icon"` üzerinde `title`): `Tooltip` + `aria-label` olmalı. Dokunmatikte `title` hiç görünmez, ekran okuyucuda da tutarsız.
- **Kesilen metin** (`truncate` + `title={tamMetin}`): Kabul edilebilir; `Tooltip` isteğe bağlı. Değiştirilmez.
- **Bileşen prop'u** (`CardTitle`, `PageHeader title=` vb.): Native ipucu değil, kapsam dışı.

#520'de yalnızca ilk grup değiştirilir. Lint kuralı `Button` üzerindeki `title`'ı uyarı (warn) olarak işaretler.

## Eklenmesi gereken shadcn bileşenleri

| Bileşen | Neden | Not |
|---|---|---|
| `table` | 10 ham tablo; `table-view` ve `editable-grid` de bunun üstüne kurulmalı | `npx shadcn add table` |
| `native-select` | Tablo içi, yoğun filtrelerde Radix `Select` ağır; mobilde native liste daha iyi | shadcn'in `native-select` bileşeni; stil token'larla uyumlu |
| `alert-dialog` | `useDialogs().confirm` şu an `Dialog` ile çiziliyor; yıkıcı onaylarda doğru rol `alertdialog` | `@radix-ui/react-alert-dialog` zaten bağımlılıklarda |
| `breadcrumb` | #521 (üst çubuk ekmek kırıntısı) ve ayarlar sayfaları | |
| `pagination` | Liste sayfalarındaki elle yazılmış önceki/sonraki düğmeleri | İsteğe bağlı, #520 kapsamı dışında kalabilir |
| `month-picker` (proje içi) | `type="month"` girişleri | `date-picker.tsx` ailesine, `Popover` + ay ızgarası |
| `drawer` | Mobilde alt sayfa; şu an `Sheet side="bottom"` kullanılıyor | Gerekli değil, `vaul` bağımlılığı eklemeye değmez |
| `hover-card` | Kullanım yeri bulunmadı | Eklenmez |

`field.tsx` (`Field`, `FieldLabel`, `FieldSet`, `FieldLegend`) zaten var; ham `label`+`fieldset` kullanımlarının çoğu buna geçer.

## Lint kuralı önerisi

`frontend/eslint.config.mjs`'deki mevcut `no-restricted-syntax` girdisine eklenir; #520'nin son PR'ında açılır:

```js
{
  files: ["src/**/*.tsx"],
  ignores: [
    "src/components/ui/**",
    "src/components/editor/**",
    "src/app/global-error.tsx",
    "src/features/legal/components/legal-markdown.tsx",
    "src/components/tables/editable-cell.tsx",
    "src/components/realtime/realtime-debug-panel.tsx",
  ],
  rules: {
    "no-restricted-syntax": [
      "error",
      // mevcut type="date" kuralı korunur
      {
        selector:
          "JSXOpeningElement[name.name=/^(button|select|textarea|table|label|details|summary|dialog|fieldset)$/]",
        message: "Native öğe yerine @/components/ui karşılığını kullanın (bkz. docs/arastirma/native-bilesen-denetimi.md).",
      },
      {
        // dosya girişleri istisna: <input type="file">
        selector:
          "JSXOpeningElement[name.name='input']:not(:has(JSXAttribute[name.name='type'][value.value='file']))",
        message: "Native <input> yerine Input, Checkbox, RadioGroup ya da DatePicker kullanın.",
      },
      {
        selector: "CallExpression[callee.object.name='window'][callee.property.name=/^(alert|confirm|prompt)$/]",
        message: "useDialogs() ya da toast kullanın.",
      },
    ],
    "no-restricted-globals": ["error", "alert", "confirm", "prompt"],
  },
}
```

Notlar:
- `no-restricted-globals` yalnızca global `confirm`'ü yakalar; `useDialogs()`'tan gelen yerel `confirm` değişkeni etkilenmez.
- ESLint'te dosya bazlı `no-restricted-syntax` girdileri üst üste binmez, sonuncusu kazanır; bu yüzden mevcut `type="date"` seçicisi aynı diziye taşınmalı.
- `invoice-preview.tsx`'teki tablolar JSX değil dize olduğundan kuraldan etkilenmez.

## #520 için önerilen PR sırası

1. Bileşen ekleme: `table`, `native-select`, `alert-dialog`, `breadcrumb`, `month-picker` (davranış değişikliği yok).
2. Ortak bileşenler: `table-view`, `editable-grid`, `form-layout`, `reorder-column`, `file-uploader`, `hakan/*`.
3. Banka (`bank-*`, `luca-bank-export-panel`).
4. Beyanname, bordro, Z raporu.
5. Mutabakat (`reconciliation-*`, `letter-response-page`).
6. Masraf, fiş, yevmiye (`window.prompt` dâhil).
7. Ofis genel + kabuk + mükellef paneli.
8. Raporlar, copilot, depolama, ayarlar, kiracı, loglar.
9. Lint kuralı (en son, CI erken kırılmasın diye) ve `useDialogs().confirm`'ün `AlertDialog`'a taşınması.

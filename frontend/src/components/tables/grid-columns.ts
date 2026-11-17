import type { ReactNode } from "react";

import type { GridEditorKind } from "@/components/tables/editable-cell";
import { parseLocaleDecimal } from "@/components/tables/grid-model";
import type { ColumnAlign } from "@/components/tables/types";

/** Düzenlenebilir ızgara kolonu (#394). */
export type EditableGridColumn<TData> = {
  id: string;
  header: string;
  /** Satırdaki özgün (kaydedilmiş) değer. */
  value: (row: TData) => unknown;
  /** Düzenleyici türü; verilmezse kolon salt okunur. */
  editor?: GridEditorKind;
  /** `select` için seçenekler; yapıştırmada değer ya da etiket eşleşir. */
  options?: { label: string; value: string }[];
  /** `text` düzenleyicide öneri listesi (ör. hesap planı kodları). */
  suggestions?: string[];
  /** Satır bazında salt okunur. */
  readOnly?: boolean | ((row: TData) => boolean);
  /** Pano/klavye metni → değer. Varsayılan düzenleyici türüne göre. */
  parse?: (text: string, row: TData) => unknown;
  /** Değer → düzenleme ve kopyalama metni. */
  toText?: (value: unknown, row: TData) => string;
  /** Hücre görünümü. Varsayılan `toText` sonucu (para için ₺ biçimi). */
  render?: (value: unknown, row: TData) => ReactNode;
  /** Hata metni ya da geçerliyse null/undefined. */
  validate?: (value: unknown, row: TData) => string | null | undefined;
  /** Boş bırakılamaz. */
  required?: boolean;
  align?: ColumnAlign;
  width?: number | string;
  placeholder?: string;
};

export type GridMessages = {
  required: string;
  invalidNumber: string;
  invalidOption: string;
};

export function isEmptyValue(v: unknown): boolean {
  return v === null || v === undefined || v === "";
}

/** Kanonik ondalık ("1234.5") → düzenleme metni ("1234,5" tr). */
export function decimalToText(value: unknown, locale: string): string {
  if (isEmptyValue(value)) return "";
  const s = String(value);
  return locale === "tr" ? s.replace(".", ",") : s;
}

const CANONICAL_DECIMAL = /^-?\d+(\.\d+)?$/;

/**
 * Görünüm için sayı biçimi (tr: 1.234,56). Para iki hane; büyük tutarlarda
 * hassasiyet kaybolmasın diye tam kısım BigInt ile gruplanır.
 */
export function formatGridNumber(
  value: unknown,
  locale: string,
  money: boolean,
): string {
  const s = String(value);
  if (!CANONICAL_DECIMAL.test(s)) return s;
  const negative = s.startsWith("-");
  const [int = "0", fracRaw = ""] = s.replace("-", "").split(".");
  const loc = locale === "tr" ? "tr-TR" : "en-GB";
  const grouped = new Intl.NumberFormat(loc).format(BigInt(int));
  const frac = money ? fracRaw.padEnd(2, "0").slice(0, 2) : fracRaw;
  const dec = locale === "tr" ? "," : ".";
  const body = frac ? `${grouped}${dec}${frac}` : grouped;
  return negative && /[1-9]/.test(s) ? `-${body}` : body;
}

export function defaultParse<TData>(
  col: EditableGridColumn<TData>,
  text: string,
): unknown {
  const t = text.trim();
  switch (col.editor) {
    case "number": {
      if (t === "") return null;
      const d = parseLocaleDecimal(t);
      // Çözülemeyen metin olduğu gibi kalır; doğrulama işaretler.
      return d === null ? t : Number(d);
    }
    case "money": {
      if (t === "") return null;
      return parseLocaleDecimal(t) ?? t;
    }
    case "select": {
      if (t === "") return null;
      const lower = t.toLocaleLowerCase("tr");
      const hit = col.options?.find(
        (o) =>
          o.value === t ||
          o.label.toLocaleLowerCase("tr") === lower ||
          o.value.toLocaleLowerCase("tr") === lower,
      );
      return hit ? hit.value : t;
    }
    default:
      return t;
  }
}

export function defaultToText<TData>(
  col: EditableGridColumn<TData>,
  value: unknown,
  locale: string,
): string {
  if (isEmptyValue(value)) return "";
  switch (col.editor) {
    case "number":
    case "money":
      return typeof value === "number" || CANONICAL_DECIMAL.test(String(value))
        ? decimalToText(value, locale)
        : String(value);
    default:
      return String(value);
  }
}

/** Görünen metin: seçenek etiketi, sayı/para metni ya da ham değer. */
export function displayText<TData>(
  col: EditableGridColumn<TData>,
  value: unknown,
  row: TData,
  locale: string,
): string {
  if (col.editor === "select" && !isEmptyValue(value)) {
    const hit = col.options?.find((o) => o.value === value);
    if (hit) return hit.label;
  }
  if (
    !col.toText &&
    (col.editor === "money" || col.editor === "number") &&
    !isEmptyValue(value) &&
    (typeof value === "number" || CANONICAL_DECIMAL.test(String(value)))
  ) {
    return formatGridNumber(value, locale, col.editor === "money");
  }
  return col.toText
    ? col.toText(value, row)
    : defaultToText(col, value, locale);
}

/** Tek hücrenin doğrulaması: zorunluluk, tür ve kolonun kendi kuralı. */
export function validateCell<TData>(
  col: EditableGridColumn<TData>,
  value: unknown,
  row: TData,
  messages: GridMessages,
): string | null {
  if (isEmptyValue(value)) {
    return col.required ? messages.required : null;
  }
  if (col.editor === "number" && typeof value !== "number") {
    return messages.invalidNumber;
  }
  if (col.editor === "money" && !CANONICAL_DECIMAL.test(String(value))) {
    return messages.invalidNumber;
  }
  if (
    col.editor === "select" &&
    col.options &&
    !col.options.some((o) => o.value === value)
  ) {
    return messages.invalidOption;
  }
  return col.validate?.(value, row) ?? null;
}

export function isColumnEditable<TData>(
  col: EditableGridColumn<TData>,
  row: TData,
): boolean {
  if (!col.editor) return false;
  if (typeof col.readOnly === "function") return !col.readOnly(row);
  return !col.readOnly;
}

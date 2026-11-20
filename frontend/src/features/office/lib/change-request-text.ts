/**
 * Kilitli dönem değişiklik talebinin alanları Türkçe (#710): `payable_amount`
 * yerine "Ödenecek tutar", `SATIS` yerine "Satış", `TRY` yerine "TL",
 * `manual_upload/ubl_upload` yerine "Elle yükleme · UBL dosyası". Saf modül.
 */

import { formatCellDate } from "@/components/tables/cell-format";
import { formatMoney } from "@/lib/decimal";
import { sourceText } from "@/lib/source-label";

import { invoiceTypeLabel } from "./ubl-codes";

type Translate = (
  key: string,
  vars?: Record<string, string | number>,
) => string;

/** Sunucunun fark özetindeki alanlar, ekrandaki sırasıyla. */
export const CHANGE_FIELDS = [
  "invoice_number",
  "issue_date",
  "period",
  "direction",
  "type",
  "customer_name",
  "customer_tax_number",
  "customer_identity_number",
  "currency",
  "tax_exclusive_amount",
  "tax_amount",
  "withholding_amount",
  "tax_inclusive_amount",
  "payable_amount",
  "item_count",
  "source",
] as const;

const AMOUNTS = new Set([
  "tax_exclusive_amount",
  "tax_amount",
  "withholding_amount",
  "tax_inclusive_amount",
  "payable_amount",
]);

/** Alanın Türkçe adı; bilinmeyen alan "Diğer bilgi" olur, ham ad görünmez. */
export function changeFieldLabel(field: string, t: Translate): string {
  const key = `office.change_requests.field_${field}`;
  const v = t(key);
  return v === key ? t("office.change_requests.field_other") : v;
}

/** Sıralama: bilinen alanlar sabit sırada, bilinmeyenler sonda. */
export function changeFieldOrder(field: string): number {
  const i = (CHANGE_FIELDS as readonly string[]).indexOf(field);
  return i < 0 ? CHANGE_FIELDS.length : i;
}

/** "2026-08" → "Ağustos 2026". */
export function periodText(period: string, locale: string): string {
  const d = new Date(`${period}-01T00:00:00`);
  if (!/^\d{4}-\d{2}$/.test(period) || Number.isNaN(d.getTime())) return period;
  return d.toLocaleDateString(locale === "tr" ? "tr-TR" : "en-GB", {
    month: "long",
    year: "numeric",
  });
}

/**
 * Alanın değerini sade metne çevirir. `value` boşsa ("—" ya da "") aynen
 * döner; `currency` tutarların para birimidir.
 */
export function changeFieldValue(
  field: string,
  value: string,
  t: Translate,
  locale: string,
  currency = "TRY",
): string {
  if (value === "" || value === "—") return value;
  if (AMOUNTS.has(field)) return formatMoney(value, currency || "TRY");
  switch (field) {
    case "direction":
      return value === "incoming" || value === "outgoing"
        ? t(`office.review_api.direction_${value}`)
        : value;
    case "type":
      return invoiceTypeLabel(value, t);
    case "currency":
      return value === "TRY" ? "TL" : value;
    case "issue_date":
      return formatCellDate(value, locale);
    case "period":
      return periodText(value, locale);
    case "source": {
      const [system, method] = value.split("/");
      return sourceText(system, method, t);
    }
    default:
      return value;
  }
}

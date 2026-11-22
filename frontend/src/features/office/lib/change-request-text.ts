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

/** Talebin özeti (`readDiff(...).summary`). */
export type ChangeSummary = {
  number: string;
  direction: string;
  counterparty: string;
  payable: string;
  currency: string;
};

/**
 * Talebin sade cümlesi (#710): "Kapalı Ağustos 2026 dönemine yeni bir satış
 * faturası geldi: GEC2026000000128, Ali Yılmaz, ₺ 492,00."
 */
export function changeSentence(
  kind: string,
  period: string,
  s: ChangeSummary,
  changed: number,
  t: Translate,
  locale: string,
): string {
  const invoice =
    s.direction === "incoming" || s.direction === "outgoing"
      ? t(`office.change_requests.invoice_${s.direction}`)
      : t("office.change_requests.invoice_any");
  const detail = [
    s.number,
    s.counterparty,
    s.payable ? formatMoney(s.payable, s.currency || "TRY") : "",
  ]
    .filter(Boolean)
    .join(", ");
  const month = periodText(period, locale);
  return kind === "create"
    ? t("office.change_requests.sentence_create", { month, invoice, detail })
    : t("office.change_requests.sentence_update", {
        month,
        invoice,
        detail,
        count: changed,
      });
}

/**
 * Karar verilmiş talebin seçeneği cümleyle (Oğuz'un demo geri bildirimi):
 * "Kasım 2026 ayına uygulandı", "Ekim 2026 dönemine düzeltme olarak
 * işlendi". Eski kayıtta karar boşsa durumdan çıkarılır.
 */
export function decisionText(
  req: {
    status: string;
    period: string;
    decision?: string | null;
    target_period?: string | null;
  },
  t: Translate,
  locale: string,
): string {
  const decision =
    req.decision ??
    (req.status === "applied"
      ? "apply"
      : req.status === "rejected"
        ? "reject"
        : "");
  if (!decision) return t("office.change_requests.decision_open");
  const month = periodText(req.period, locale);
  const target = req.target_period ? periodText(req.target_period, locale) : "";
  return t(`office.change_requests.decision_${decision}`, { month, target });
}

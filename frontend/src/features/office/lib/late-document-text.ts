/**
 * Geç gelen belge cümleleri (#780 C). Saf modül: belgenin geliş tarihi ile
 * ait olduğu dönem ayrı gösterilir; "Bu belge dönem kapandıktan sonra geldi
 * (Ekim 2026 kapanışı: 3 Kasım; geliş: 18 Kasım)".
 */

import { periodText } from "./change-request-text";

type Translate = (
  key: string,
  vars?: Record<string, string | number>,
) => string;

export const LATE_STATUSES = [
  "pending",
  "correction",
  "current_period",
  "ignored",
] as const;

export type LateStatus = (typeof LATE_STATUSES)[number];

/** Gün ve ay, İstanbul saatine göre: "2026-11-03T07:00:00Z" → "3 Kasım". */
export function dayMonth(iso: string, locale: string): string {
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return iso;
  return d.toLocaleDateString(locale === "tr" ? "tr-TR" : "en-GB", {
    day: "numeric",
    month: "long",
    timeZone: "Europe/Istanbul",
  });
}

/** Uyarı cümlesi; kapanış nedeni (kilit / beyan) ayrı cümlededir. */
export function lateSentence(
  item: { period: string; closed_at: string; arrived_at: string },
  t: Translate,
  locale: string,
): string {
  return t("office.late_docs.sentence", {
    month: periodText(item.period, locale),
    closed: dayMonth(item.closed_at, locale),
    arrived: dayMonth(item.arrived_at, locale),
  });
}

/** Durum rozetinin tonu. */
export function lateStatusTone(
  status: string,
): "warn" | "info" | "ok" | "neutral" {
  switch (status) {
    case "pending":
      return "warn";
    case "correction":
      return "info";
    case "current_period":
      return "ok";
    default:
      return "neutral";
  }
}

/** Belge türünün adı: Fatura, Masraf fişi, Z raporu. */
export function lateDocumentKind(type: string, t: Translate): string {
  const key = `office.late_docs.kind_${type}`;
  const v = t(key);
  return v === key ? t("office.late_docs.kind_other") : v;
}

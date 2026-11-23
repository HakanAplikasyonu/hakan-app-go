/**
 * Evrak yüklemede dönem önerisi (Oğuz'un demo geri bildirimi): belge tarihi
 * biliniyorsa (kullanıcı girdi ya da e-Fatura XML'inden okundu) onun ayı,
 * yoksa içinde bulunulan ay (Türkiye saati).
 */

/** "2026-10-31" → "2026-10"; geçersizse null. */
export function monthOfDate(date: string | null | undefined): string | null {
  if (!date || !/^\d{4}-(0[1-9]|1[0-2])-\d{2}$/.test(date)) return null;
  return date.slice(0, 7);
}

/** İçinde bulunulan ay, Türkiye saatiyle (YYYY-MM). */
export function currentMonth(now: Date = new Date()): string {
  const parts = new Intl.DateTimeFormat("en-CA", {
    timeZone: "Europe/Istanbul",
    year: "numeric",
    month: "2-digit",
  }).formatToParts(now);
  const y = parts.find((p) => p.type === "year")?.value ?? "1970";
  const m = parts.find((p) => p.type === "month")?.value ?? "01";
  return `${y}-${m}`;
}

export type PeriodSuggestion = {
  period: string;
  source: "document_date" | "current_month";
};

/** Belge tarihinden ay önerisi; tarih yoksa içinde bulunulan ay. */
export function suggestPeriod(
  documentDate: string | null | undefined,
  now: Date = new Date(),
): PeriodSuggestion {
  const m = monthOfDate(documentDate);
  if (m) return { period: m, source: "document_date" };
  return { period: currentMonth(now), source: "current_month" };
}

/** e-Fatura UBL XML metninden düzenleme tarihi (cbc:IssueDate). */
export function ublIssueDate(xml: string): string | null {
  const m = /<(?:[A-Za-z0-9_-]+:)?IssueDate>\s*(\d{4}-\d{2}-\d{2})\s*</.exec(
    xml,
  );
  return m && monthOfDate(m[1]) ? m[1] : null;
}

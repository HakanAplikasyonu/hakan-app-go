/**
 * Global dönem seçici (#326): ofis panelindeki bütün listeler tek bir muhasebe
 * dönemine (YYYY-MM) göre süzülür. Backend sözleşmesi `period` sorgu
 * parametresi (#333, `periodq`): geçersiz değer 400 INVALID_PERIOD.
 */

/** "Tüm dönemler": listeler süzülmez (`period` gönderilmez). */
export const ALL_PERIODS = "all";

export type PeriodChoice = typeof ALL_PERIODS | string;

const PERIOD_RE = /^(19|20)\d{2}-(0[1-9]|1[0-2])$/;

export function isPeriod(value: unknown): value is string {
  return typeof value === "string" && PERIOD_RE.test(value);
}

/** Ofis saatiyle (Europe/Istanbul) bugünün ayı. */
export function currentPeriod(now = new Date()) {
  const parts = new Intl.DateTimeFormat("en-CA", {
    timeZone: "Europe/Istanbul",
    year: "numeric",
    month: "2-digit",
  }).formatToParts(now);
  const y = parts.find((p) => p.type === "year")?.value ?? "1970";
  const m = parts.find((p) => p.type === "month")?.value ?? "01";
  return `${y}-${m}`;
}

export function shiftPeriod(period: string, months: number) {
  const [y, m] = period.split("-").map(Number);
  const d = new Date(Date.UTC(y, m - 1 + months, 1));
  return `${d.getUTCFullYear()}-${String(d.getUTCMonth() + 1).padStart(2, "0")}`;
}

/** Beyan dönemi: bir önceki ay (beyannamesi bu ay veriliyor). */
export function declarationPeriod(now = new Date()) {
  return shiftPeriod(currentPeriod(now), -1);
}

/** Seçicide gösterilen dönemler: bu ay ve geriye doğru `count - 1` ay. */
export function recentPeriods(count = 18, now = new Date()) {
  const head = currentPeriod(now);
  return Array.from({ length: count }, (_, i) => shiftPeriod(head, -i));
}

/** KDV beyannamesi son günü: dönemi izleyen ayın 28'i. */
export function vatDueDate(period: string) {
  const next = shiftPeriod(period, 1);
  return `${next}-28`;
}

/** Bugünden (ofis saati) `date` (YYYY-MM-DD) gününe kalan gün. */
export function daysUntil(date: string, now = new Date()) {
  const today = new Intl.DateTimeFormat("en-CA", {
    timeZone: "Europe/Istanbul",
  }).format(now);
  const a = Date.parse(`${today}T00:00:00Z`);
  const b = Date.parse(`${date}T00:00:00Z`);
  return Math.round((b - a) / 86_400_000);
}

export function periodLabel(period: string, locale: string) {
  const [y, m] = period.split("-").map(Number);
  return new Intl.DateTimeFormat(locale === "tr" ? "tr-TR" : "en-GB", {
    month: "long",
    year: "numeric",
    timeZone: "UTC",
  }).format(new Date(Date.UTC(y, m - 1, 1)));
}

export function monthName(period: string, locale: string) {
  const [y, m] = period.split("-").map(Number);
  return new Intl.DateTimeFormat(locale === "tr" ? "tr-TR" : "en-GB", {
    month: "long",
    timeZone: "UTC",
  }).format(new Date(Date.UTC(y, m - 1, 1)));
}

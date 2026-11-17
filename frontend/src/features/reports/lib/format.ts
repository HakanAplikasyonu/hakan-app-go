/** Raporun ondalık dizgelerini grafik sayısına çevirir (yalnızca çizim). */
export function num(value: string | null | undefined): number {
  const n = Number(value);
  return Number.isFinite(n) ? n : 0;
}

function intlLocale(locale: string) {
  return locale === "tr" ? "tr-TR" : "en-GB";
}

/** ₺ 12.345,67 */
export function money(value: string | null | undefined, locale: string) {
  return new Intl.NumberFormat(intlLocale(locale), {
    style: "currency",
    currency: "TRY",
    minimumFractionDigits: 2,
    maximumFractionDigits: 2,
  }).format(num(value));
}

/** Eksen için kısa tutar: 1,2 Mn, 450 B. */
export function compact(value: number, locale: string) {
  return new Intl.NumberFormat(intlLocale(locale), {
    notation: "compact",
    maximumFractionDigits: 1,
  }).format(value);
}

/** "18.8" → "+%18,8" */
export function percent(value: string | null | undefined, locale: string) {
  if (value === null || value === undefined) return null;
  const n = num(value);
  const body = new Intl.NumberFormat(intlLocale(locale), {
    maximumFractionDigits: 1,
    minimumFractionDigits: 1,
  }).format(Math.abs(n));
  const sign = n > 0 ? "+" : n < 0 ? "−" : "";
  return locale === "tr" ? `${sign}%${body}` : `${sign}${body}%`;
}

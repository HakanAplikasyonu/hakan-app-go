/** Amounts arrive as decimal strings; format them for display only. */
export function formatMoney(
  value: string | undefined,
  currency: string,
  locale: string,
) {
  if (value === undefined || value === "") return "—";
  const n = Number(value);
  if (!Number.isFinite(n)) return value;
  try {
    return new Intl.NumberFormat(locale, {
      style: "currency",
      currency: currency || "TRY",
      minimumFractionDigits: 2,
      maximumFractionDigits: 2,
    }).format(n);
  } catch {
    return `${n.toFixed(2)} ${currency}`;
  }
}

export function formatDate(value: string | undefined, locale: string) {
  if (!value) return "—";
  const d = new Date(value.length === 10 ? `${value}T00:00:00` : value);
  if (Number.isNaN(d.getTime())) return value;
  return new Intl.DateTimeFormat(locale, { dateStyle: "medium" }).format(d);
}

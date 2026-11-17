import { formatMoney } from "@/lib/decimal";

/** Tablo hücrelerinde tarih biçimleri (#381). */
export type CellDateStyle = "short" | "medium" | "datetime";

/** Uygulama dilinden Intl yerel ayarı: tr → tr-TR, diğerleri en-GB. */
export function intlLocale(locale: string): string {
  return locale === "tr" ? "tr-TR" : "en-GB";
}

/**
 * Tarih hücresi metni. Yalnızca gün olan değerler ("2026-11-17") yerel gece
 * yarısına sabitlenir; böylece saat dilimi farkı bir önceki güne kaydırmaz.
 * Boş değer için `empty`, çözülemeyen değer için ham metin döner.
 */
export function formatCellDate(
  value: string | Date | null | undefined,
  locale: string,
  style: CellDateStyle = "short",
  empty = "—",
): string {
  if (value === null || value === undefined || value === "") return empty;
  const d =
    value instanceof Date
      ? value
      : new Date(
          /^\d{4}-\d{2}-\d{2}$/.test(value) ? `${value}T00:00:00` : value,
        );
  if (Number.isNaN(d.getTime())) return String(value);
  const loc = intlLocale(locale);
  if (style === "medium") {
    return new Intl.DateTimeFormat(loc, { dateStyle: "medium" }).format(d);
  }
  if (style === "datetime") {
    return new Intl.DateTimeFormat(loc, {
      dateStyle: "short",
      timeStyle: "short",
    }).format(d);
  }
  return d.toLocaleDateString(loc);
}

/**
 * Para hücresi metni: ondalık string, kuruş hassasiyetinde ve her zaman
 * tr-TR gruplamasıyla ("₺ 1.284.560,40"). Boş değer için `empty` döner.
 */
export function formatCellMoney(
  value: string | number | null | undefined,
  currency = "TRY",
  empty = "—",
): string {
  if (value === null || value === undefined || value === "") return empty;
  const s = typeof value === "number" ? value.toFixed(2) : value.trim();
  if (!/^[-+]?\d+(\.\d+)?$/.test(s)) return String(value);
  return formatMoney(s, currency || "TRY");
}

/** Ondalık string karşılaştırması (sıralama için; float yuvarlaması yok). */
export function compareDecimal(
  a: string | number | null | undefined,
  b: string | number | null | undefined,
): number {
  const na = Number(a ?? 0);
  const nb = Number(b ?? 0);
  if (Number.isNaN(na) || Number.isNaN(nb))
    return String(a).localeCompare(String(b));
  return na === nb ? 0 : na < nb ? -1 : 1;
}

/** ISO tarih karşılaştırması; boş değerler sona. */
export function compareDate(
  a: string | null | undefined,
  b: string | null | undefined,
): number {
  if (!a && !b) return 0;
  if (!a) return 1;
  if (!b) return -1;
  const ta = new Date(a).getTime();
  const tb = new Date(b).getTime();
  return ta === tb ? 0 : ta < tb ? -1 : 1;
}

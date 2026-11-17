/**
 * Mükellef detayının sekme ve geri dönüş yardımcıları (#468): saf
 * fonksiyonlar, birim testli.
 */

function safeDecode(p: string) {
  try {
    return decodeURIComponent(p);
  } catch {
    return p;
  }
}

/**
 * Sekmenin `aria-current` değeri: tam eşleşmede `page`, sekmenin altındaki
 * bir detayda (`/invoices/{id}`) `true`; `exact` sekme (genel bakış) yalnızca
 * tam eşleşmede etkin.
 */
export function tabState(
  pathname: string,
  tab: { href: string; exact?: boolean },
): "page" | "true" | undefined {
  const path = safeDecode(pathname).replace(/\/+$/, "");
  const base = safeDecode(tab.href.split("?")[0] ?? "").replace(/\/+$/, "");
  if (path === base) return "page";
  if (!tab.exact && path.startsWith(`${base}/`)) return "true";
  return undefined;
}

/**
 * Liste ile detay arasında taşınan arama parametreleri (süzgeç, sayfa).
 * Çekmece (`invoice`) ve mükellef (`taxpayer`) parametreleri taşınmaz:
 * mükellef rotada sabit.
 */
export function carryListParams(
  params: { toString(): string },
  drop: string[] = [],
): string {
  const next = new URLSearchParams(params.toString());
  for (const k of ["invoice", "taxpayer", ...drop]) next.delete(k);
  const qs = next.toString();
  return qs ? `?${qs}` : "";
}

/**
 * Üst çubuğun bağlamı (#521): adresten ofis kökü, açık mükellef ve sayfa.
 * Saf işlevler; ekmek kırıntısı ve hızlı mükellef değiştirici bunları kullanır.
 */

const enc = (s: string) => encodeURIComponent(s);

/** `/t/{slug}` kökünden sonraki yol parçaları. Kök dışındaysa null. */
function segmentsAfterRoot(pathname: string, slug: string): string[] | null {
  const parts = pathname.split("/").filter(Boolean);
  if (parts[0] !== "t" || decodeURIComponent(parts[1] ?? "") !== slug)
    return null;
  return parts.slice(2);
}

/** Adresteki açık mükellefin (engagement) kimliği; yoksa null. */
export function engagementInPath(pathname: string, slug: string) {
  const rest = segmentsAfterRoot(pathname, slug);
  if (!rest || rest[0] !== "engagements" || !rest[1]) return null;
  return decodeURIComponent(rest[1]);
}

/**
 * Aynı sayfayı başka mükellef için açan adres: mükellef değişir, sekme
 * kalır (kayıt ayrıntısı gibi alt yollar sekmenin köküne döner).
 */
export function swapEngagement(pathname: string, slug: string, to: string) {
  const rest = segmentsAfterRoot(pathname, slug);
  const base = `/t/${enc(slug)}/engagements/${enc(to)}`;
  if (!rest || rest[0] !== "engagements") return base;
  const tab = rest[2];
  return tab ? `${base}/${tab}` : base;
}

/**
 * Adrese en uzun eşleşen bağlantı: `/a/b` adresi `/a` ve `/a/b` arasından
 * `/a/b`'yi seçer. `exact` olan yalnızca birebir eşleşir.
 */
export function matchByPath<T extends { href: string; exact?: boolean }>(
  pathname: string,
  items: T[],
): T | undefined {
  const path = pathname.replace(/\/+$/, "") || "/";
  let best: T | undefined;
  for (const item of items) {
    const href = item.href.split("?")[0]!.replace(/\/+$/, "") || "/";
    const hit = item.exact
      ? path === href
      : path === href || path.startsWith(`${href}/`);
    if (hit && (!best || href.length > best.href.split("?")[0]!.length))
      best = item;
  }
  return best;
}

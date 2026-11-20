import { FOCUS_PARAM } from "./related-href";

/**
 * Seçili kaydın listedeki komşusu (#564): sayfadaki `?kayit=` bağlantıları
 * (liste satırları, belge sırasıyla) içinde aynı türden önceki / sonraki
 * kaydın adresi. Yoksa null (listenin başı ya da sonu, ya da kayıt bu
 * sayfada listelenmiyor).
 */
export function focusNeighbor(
  hrefs: string[],
  current: { kind: string; uuid: string },
  dir: 1 | -1,
): string | null {
  const sameKind = `${FOCUS_PARAM}=${current.kind}:`;
  const seen = new Set<string>();
  const list: string[] = [];
  for (const raw of hrefs) {
    let href = raw;
    try {
      href = decodeURIComponent(raw);
    } catch {
      // Bozuk kaçış: olduğu gibi karşılaştırılır.
    }
    const at = href.indexOf(sameKind);
    if (at < 0) continue;
    const id = href.slice(at + sameKind.length).split(/[&#]/)[0] ?? "";
    if (!id || seen.has(id)) continue;
    seen.add(id);
    list.push(raw);
  }
  const ids = [...seen];
  const at = ids.indexOf(current.uuid);
  if (at < 0) return null;
  return list[at + dir] ?? null;
}

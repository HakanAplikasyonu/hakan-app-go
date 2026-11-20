/**
 * Panelin listedeki yeri: açık uyarı listede yoksa (çözülüp düştüyse) son
 * açılan gösterilir ve onun yerine geçen kayıt "sıradaki" olur.
 */
export function sheetPosition<T extends { uuid: string }>(
  items: T[],
  openUuid: string | null,
  last: { warning: T; index: number } | null,
) {
  const found = openUuid ? items.findIndex((w) => w.uuid === openUuid) : -1;
  const current =
    found >= 0
      ? items[found]
      : last && last.warning.uuid === openUuid
        ? last.warning
        : null;
  const gone = found < 0 && current !== null;
  const index = found >= 0 ? found : Math.min(last?.index ?? 0, items.length);
  const prev = index > 0 ? items[index - 1] : undefined;
  const next = gone ? items[index] : items[index + 1];
  // Çözülüp düşen uyarının yerine panelin kendiliğinden açacağı kayıt (#570):
  // yerine geçen, o yoksa (sondaydı) bir önceki; liste boşsa yok.
  const successor = gone ? (items[index] ?? items[index - 1]) : undefined;
  return { current, gone, index, prev, next, successor };
}

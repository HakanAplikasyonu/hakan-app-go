/**
 * Düzenlenebilir ızgara (#394) için saf model: seçim, klavye gezintisi, TSV
 * yapıştırma/kopyalama, doldurma, geri al/yinele, doğrulama ve satır
 * taslakları. React'ten bağımsızdır; birim testleri `grid-model.test.ts`.
 *
 * Koordinatlar görünür sıraya göredir: `row` veri satırı sırası, `col`
 * kolon sırası (0 tabanlı).
 */

export type CellCoord = { row: number; col: number };

/** Seçim: çapa (ilk tıklanan) ve odak (etkin hücre). */
export type CellRange = { anchor: CellCoord; focus: CellCoord };

export type GridBounds = { rows: number; cols: number };

export type NormalizedRange = {
  top: number;
  bottom: number;
  left: number;
  right: number;
};

export function clampCoord(c: CellCoord, b: GridBounds): CellCoord {
  return {
    row: Math.min(Math.max(c.row, 0), Math.max(b.rows - 1, 0)),
    col: Math.min(Math.max(c.col, 0), Math.max(b.cols - 1, 0)),
  };
}

export function normalizeRange(r: CellRange): NormalizedRange {
  return {
    top: Math.min(r.anchor.row, r.focus.row),
    bottom: Math.max(r.anchor.row, r.focus.row),
    left: Math.min(r.anchor.col, r.focus.col),
    right: Math.max(r.anchor.col, r.focus.col),
  };
}

export function rangeContains(r: NormalizedRange, c: CellCoord): boolean {
  return (
    c.row >= r.top && c.row <= r.bottom && c.col >= r.left && c.col <= r.right
  );
}

export function rangeSize(r: NormalizedRange): { rows: number; cols: number } {
  return { rows: r.bottom - r.top + 1, cols: r.right - r.left + 1 };
}

export function singleRange(c: CellCoord): CellRange {
  return { anchor: c, focus: c };
}

export type NavKey =
  | "ArrowUp"
  | "ArrowDown"
  | "ArrowLeft"
  | "ArrowRight"
  | "Home"
  | "End"
  | "PageUp"
  | "PageDown"
  | "Tab"
  | "Enter";

const NAV_KEYS = new Set<string>([
  "ArrowUp",
  "ArrowDown",
  "ArrowLeft",
  "ArrowRight",
  "Home",
  "End",
  "PageUp",
  "PageDown",
  "Tab",
  "Enter",
]);

export function isNavKey(key: string): key is NavKey {
  return NAV_KEYS.has(key);
}

export type NavModifiers = {
  shift?: boolean;
  /** Ctrl (Windows/Linux) veya Cmd (macOS): kenara atla. */
  ctrl?: boolean;
  /** Sayfa kaydırma adımı (PageUp/PageDown). */
  pageSize?: number;
  /** Yazı yönü sağdan sola ise sol/sağ yer değiştirir. */
  rtl?: boolean;
};

/**
 * Klavye tuşuna göre bir sonraki hücre. Tab satır sonunda bir alt satırın
 * başına sarar (Shift+Tab tersine); Enter aşağı, Shift+Enter yukarı.
 */
export function moveCoord(
  c: CellCoord,
  key: NavKey,
  b: GridBounds,
  mods: NavModifiers = {},
): CellCoord {
  if (b.rows === 0 || b.cols === 0) return c;
  const lastRow = b.rows - 1;
  const lastCol = b.cols - 1;
  const page = Math.max(mods.pageSize ?? 10, 1);
  let left: "ArrowLeft" | "ArrowRight" = "ArrowLeft";
  let right: "ArrowLeft" | "ArrowRight" = "ArrowRight";
  if (mods.rtl) [left, right] = [right, left];
  switch (key) {
    case "ArrowUp":
      return clampCoord({ row: mods.ctrl ? 0 : c.row - 1, col: c.col }, b);
    case "ArrowDown":
      return clampCoord(
        { row: mods.ctrl ? lastRow : c.row + 1, col: c.col },
        b,
      );
    case left:
      return clampCoord({ row: c.row, col: mods.ctrl ? 0 : c.col - 1 }, b);
    case right:
      return clampCoord(
        { row: c.row, col: mods.ctrl ? lastCol : c.col + 1 },
        b,
      );
    case "Home":
      return mods.ctrl ? { row: 0, col: 0 } : { row: c.row, col: 0 };
    case "End":
      return mods.ctrl
        ? { row: lastRow, col: lastCol }
        : { row: c.row, col: lastCol };
    case "PageUp":
      return clampCoord({ row: c.row - page, col: c.col }, b);
    case "PageDown":
      return clampCoord({ row: c.row + page, col: c.col }, b);
    case "Tab":
      if (mods.shift) {
        if (c.col > 0) return { row: c.row, col: c.col - 1 };
        return c.row > 0 ? { row: c.row - 1, col: lastCol } : c;
      }
      if (c.col < lastCol) return { row: c.row, col: c.col + 1 };
      return c.row < lastRow ? { row: c.row + 1, col: 0 } : c;
    case "Enter":
      return clampCoord({ row: c.row + (mods.shift ? -1 : 1), col: c.col }, b);
    default:
      return c;
  }
}

/**
 * Excel/Sheets panosundaki TSV metnini matrise çevirir. Tırnaklı alanlar
 * (içinde sekme, satır sonu ya da `""` kaçışı) desteklenir; sondaki tek
 * satır sonu yok sayılır. CRLF ve LF ikisi de satır ayırıcıdır.
 */
export function parseTsv(text: string): string[][] {
  if (text === "") return [];
  const rows: string[][] = [];
  let row: string[] = [];
  let field = "";
  let i = 0;
  let quoted = false;
  let fieldStart = true;
  while (i < text.length) {
    const ch = text[i]!;
    if (quoted) {
      if (ch === '"') {
        if (text[i + 1] === '"') {
          field += '"';
          i += 2;
          continue;
        }
        quoted = false;
        i += 1;
        continue;
      }
      field += ch;
      i += 1;
      continue;
    }
    if (ch === '"' && fieldStart) {
      quoted = true;
      fieldStart = false;
      i += 1;
      continue;
    }
    if (ch === "\t") {
      row.push(field);
      field = "";
      fieldStart = true;
      i += 1;
      continue;
    }
    if (ch === "\r" || ch === "\n") {
      row.push(field);
      rows.push(row);
      row = [];
      field = "";
      fieldStart = true;
      i += ch === "\r" && text[i + 1] === "\n" ? 2 : 1;
      continue;
    }
    field += ch;
    fieldStart = false;
    i += 1;
  }
  // Son satır: metin satır sonuyla bitmediyse ya da yarım alan kaldıysa.
  if (!fieldStart || field !== "" || row.length > 0) {
    row.push(field);
    rows.push(row);
  }
  return rows;
}

/** Matrisi Excel'in okuyacağı TSV'ye çevirir (gerekirse tırnaklar). */
export function toTsv(matrix: string[][]): string {
  return matrix
    .map((r) =>
      r
        .map((v) => (/[\t\n\r"]/.test(v) ? `"${v.replace(/"/g, '""')}"` : v))
        .join("\t"),
    )
    .join("\n");
}

/**
 * tr-TR / en biçimli sayı metnini kanonik ondalık metne çevirir
 * ("1.234,56" → "1234.56", "₺ 1.234" → "1234", "(12,5)" → "-12.5").
 * Çözülemezse null. Tek ayırıcı nokta + tam üç hane binlik sayılır
 * ("1.500" → "1500"); Türkçe Excel çıktısıyla uyumlu.
 */
export function parseLocaleDecimal(input: string): string | null {
  let s = input.trim().replace(/[\s ₺$€£]|TL|TRY/gi, "");
  if (s === "") return null;
  let negative = false;
  if (/^\(.*\)$/.test(s)) {
    negative = true;
    s = s.slice(1, -1);
  }
  if (s.startsWith("-")) {
    negative = !negative;
    s = s.slice(1);
  } else if (s.startsWith("+")) {
    s = s.slice(1);
  }
  if (!/^[0-9.,]+$/.test(s) || !/[0-9]/.test(s)) return null;
  const lastDot = s.lastIndexOf(".");
  const lastComma = s.lastIndexOf(",");
  let intPart: string;
  let frac = "";
  if (lastDot >= 0 && lastComma >= 0) {
    const dec = lastDot > lastComma ? "." : ",";
    const group = dec === "." ? "," : ".";
    const at = s.lastIndexOf(dec);
    intPart = s.slice(0, at).split(group).join("");
    frac = s.slice(at + 1);
    if (/[.,]/.test(intPart) || /[.,]/.test(frac)) return null;
  } else if (lastComma >= 0) {
    const parts = s.split(",");
    if (parts.length > 2) {
      if (!parts.slice(1).every((p) => p.length === 3)) return null;
      intPart = parts.join("");
    } else {
      intPart = parts[0]!;
      frac = parts[1]!;
    }
  } else if (lastDot >= 0) {
    const parts = s.split(".");
    const grouped = parts.slice(1).every((p) => p.length === 3);
    if (parts.length > 2) {
      if (!grouped) return null;
      intPart = parts.join("");
    } else if (grouped && parts[0] !== "" && parts[0] !== "0") {
      intPart = parts.join("");
    } else {
      intPart = parts[0]!;
      frac = parts[1]!;
    }
  } else {
    intPart = s;
  }
  if (intPart === "") intPart = "0";
  if (!/^[0-9]+$/.test(intPart) || (frac !== "" && !/^[0-9]+$/.test(frac))) {
    return null;
  }
  intPart = intPart.replace(/^0+(?=\d)/, "");
  frac = frac.replace(/0+$/, "");
  const body = frac ? `${intPart}.${frac}` : intPart;
  return negative && /[1-9]/.test(body) ? `-${body}` : body;
}

// ---------------------------------------------------------------------------
// Değişiklikler, taslaklar, geçmiş

/** Tek hücre değişikliği (geri alınabilir birim). */
export type CellChange = {
  rowId: string;
  columnId: string;
  prev: unknown;
  next: unknown;
};

/** Satır kimliği → kolon kimliği → kaydedilmemiş değer. */
export type GridDrafts = Readonly<
  Record<string, Readonly<Record<string, unknown>>>
>;

export function sameValue(a: unknown, b: unknown): boolean {
  if (Object.is(a, b)) return true;
  const empty = (v: unknown) => v === null || v === undefined || v === "";
  if (empty(a) && empty(b)) return true;
  if (typeof a === "number" || typeof b === "number") {
    return String(a) === String(b);
  }
  return false;
}

/**
 * Değişiklikleri taslaklara uygular. Özgün değerine dönen hücre taslaktan
 * düşer; boşalan satır da kaldırılır (satır yeniden "temiz" olur).
 */
export function applyChanges(
  drafts: GridDrafts,
  changes: readonly CellChange[],
  original: (rowId: string, columnId: string) => unknown,
  direction: "forward" | "backward" = "forward",
): GridDrafts {
  if (changes.length === 0) return drafts;
  const out: Record<string, Record<string, unknown>> = {};
  for (const [k, v] of Object.entries(drafts)) out[k] = { ...v };
  for (const ch of changes) {
    const value = direction === "forward" ? ch.next : ch.prev;
    const row = (out[ch.rowId] ??= {});
    if (sameValue(value, original(ch.rowId, ch.columnId))) {
      delete row[ch.columnId];
    } else {
      row[ch.columnId] = value;
    }
    if (Object.keys(row).length === 0) delete out[ch.rowId];
  }
  return out;
}

export type GridHistory = {
  past: readonly (readonly CellChange[])[];
  future: readonly (readonly CellChange[])[];
};

export const EMPTY_HISTORY: GridHistory = { past: [], future: [] };

/** Geçmişte tutulan en fazla işlem (bellek sınırı). */
export const HISTORY_LIMIT = 200;

export function pushHistory(
  h: GridHistory,
  batch: readonly CellChange[],
  limit = HISTORY_LIMIT,
): GridHistory {
  if (batch.length === 0) return h;
  const past = [...h.past, batch];
  return { past: past.slice(Math.max(past.length - limit, 0)), future: [] };
}

export function undoHistory(
  h: GridHistory,
): { history: GridHistory; batch: readonly CellChange[] } | null {
  const batch = h.past[h.past.length - 1];
  if (!batch) return null;
  return {
    history: { past: h.past.slice(0, -1), future: [batch, ...h.future] },
    batch,
  };
}

export function redoHistory(
  h: GridHistory,
): { history: GridHistory; batch: readonly CellChange[] } | null {
  const batch = h.future[0];
  if (!batch) return null;
  return {
    history: { past: [...h.past, batch], future: h.future.slice(1) },
    batch,
  };
}

/** Kaydedilen satırın değişikliklerini geçmişten çıkarır (artık geri alınmaz). */
export function forgetRow(h: GridHistory, rowId: string): GridHistory {
  const strip = (list: readonly (readonly CellChange[])[]) =>
    list
      .map((b) => b.filter((c) => c.rowId !== rowId))
      .filter((b) => b.length > 0);
  return { past: strip(h.past), future: strip(h.future) };
}

// ---------------------------------------------------------------------------
// Yapıştırma ve doldurma

/** Modelin kolon hakkında bilmesi gerekenler (bileşen kolonundan türetilir). */
export type GridColumnModel = {
  id: string;
  /** Bu satırda düzenlenebilir mi? */
  editable: (rowId: string) => boolean;
  /** Pano/klavye metnini değere çevirir. */
  parse: (text: string, rowId: string) => unknown;
};

export type GridModelInput = {
  rowIds: readonly string[];
  columns: readonly GridColumnModel[];
  /** Hücrenin şu anki (taslak dahil) değeri. */
  valueAt: (rowId: string, columnId: string) => unknown;
};

/**
 * Pano matrisini yapıştırır. Seçim tek hücreyse matris oradan başlayarak
 * sığdığı kadar yazılır; seçim daha büyük ve matris seçime tam bölünüyorsa
 * (ör. tek değer → tüm seçim) matris seçim boyunca tekrarlanır. Salt okunur
 * hücreler atlanır.
 */
export function pasteChanges(
  input: GridModelInput,
  selection: NormalizedRange,
  matrix: string[][],
): CellChange[] {
  if (matrix.length === 0) return [];
  const mRows = matrix.length;
  const mCols = Math.max(...matrix.map((r) => r.length));
  const sel = rangeSize(selection);
  const tile =
    (sel.rows > 1 || sel.cols > 1) &&
    sel.rows % mRows === 0 &&
    sel.cols % mCols === 0;
  const rows = tile ? sel.rows : mRows;
  const cols = tile ? sel.cols : mCols;
  const changes: CellChange[] = [];
  for (let r = 0; r < rows; r++) {
    const rowId = input.rowIds[selection.top + r];
    if (rowId === undefined) break;
    for (let c = 0; c < cols; c++) {
      const col = input.columns[selection.left + c];
      if (!col) break;
      const text = matrix[r % mRows]?.[c % mCols];
      if (text === undefined || !col.editable(rowId)) continue;
      const prev = input.valueAt(rowId, col.id);
      const next = col.parse(text, rowId);
      if (!sameValue(prev, next)) {
        changes.push({ rowId, columnId: col.id, prev, next });
      }
    }
  }
  return changes;
}

/**
 * Doldurma: `source` aralığındaki değerleri `target` (kaynağı içeren, aynı
 * kolon ya da satır hizasında genişletilmiş aralık) boyunca tekrarlar.
 * Ctrl+D = ilk satırı aşağı, Ctrl+R = ilk kolonu sağa doldurmanın özel hali.
 */
export function fillChanges(
  input: GridModelInput,
  source: NormalizedRange,
  target: NormalizedRange,
): CellChange[] {
  const src = rangeSize(source);
  const changes: CellChange[] = [];
  for (let r = target.top; r <= target.bottom; r++) {
    const rowId = input.rowIds[r];
    if (rowId === undefined) continue;
    for (let c = target.left; c <= target.right; c++) {
      if (rangeContains(source, { row: r, col: c })) continue;
      const col = input.columns[c];
      if (!col || !col.editable(rowId)) continue;
      const sr = source.top + mod(r - source.top, src.rows);
      const sc = source.left + mod(c - source.left, src.cols);
      const srcRow = input.rowIds[sr];
      const srcCol = input.columns[sc];
      if (srcRow === undefined || !srcCol) continue;
      const next = input.valueAt(srcRow, srcCol.id);
      const prev = input.valueAt(rowId, col.id);
      if (!sameValue(prev, next)) {
        changes.push({ rowId, columnId: col.id, prev, next });
      }
    }
  }
  return changes;
}

function mod(n: number, m: number) {
  return ((n % m) + m) % m;
}

/** Ctrl+D: seçimin ilk satırını alttaki satırlara kopyalar. */
export function fillDownChanges(
  input: GridModelInput,
  sel: NormalizedRange,
): CellChange[] {
  if (sel.bottom === sel.top) return [];
  return fillChanges(input, { ...sel, bottom: sel.top }, sel);
}

/** Ctrl+R: seçimin ilk kolonunu sağdaki kolonlara kopyalar. */
export function fillRightChanges(
  input: GridModelInput,
  sel: NormalizedRange,
): CellChange[] {
  if (sel.right === sel.left) return [];
  return fillChanges(input, { ...sel, right: sel.left }, sel);
}

/**
 * Doldurma tutamacı sürüklemesi: kaynak seçim ve sürüklenen hücreden hedef
 * aralık. Excel gibi baskın eksende (dikey ya da yatay) genişler.
 */
export function fillTarget(
  source: NormalizedRange,
  over: CellCoord,
): NormalizedRange {
  const down = over.row - source.bottom;
  const up = source.top - over.row;
  const right = over.col - source.right;
  const left = source.left - over.col;
  const vertical = Math.max(down, up, 0);
  const horizontal = Math.max(right, left, 0);
  if (vertical === 0 && horizontal === 0) return source;
  if (vertical >= horizontal) {
    return down > 0
      ? { ...source, bottom: over.row }
      : { ...source, top: over.row };
  }
  return right > 0
    ? { ...source, right: over.col }
    : { ...source, left: over.col };
}

/** Seçili düzenlenebilir hücreleri boşaltır (Delete / Backspace / Kes). */
export function clearChanges(
  input: GridModelInput,
  sel: NormalizedRange,
): CellChange[] {
  const changes: CellChange[] = [];
  for (let r = sel.top; r <= sel.bottom; r++) {
    const rowId = input.rowIds[r];
    if (rowId === undefined) continue;
    for (let c = sel.left; c <= sel.right; c++) {
      const col = input.columns[c];
      if (!col || !col.editable(rowId)) continue;
      const prev = input.valueAt(rowId, col.id);
      const next = col.parse("", rowId);
      if (!sameValue(prev, next)) {
        changes.push({ rowId, columnId: col.id, prev, next });
      }
    }
  }
  return changes;
}

/** Seçimi pano matrisine çevirir (Ctrl+C). */
export function copyMatrix(
  input: GridModelInput,
  sel: NormalizedRange,
  toText: (rowId: string, columnId: string, value: unknown) => string,
): string[][] {
  const out: string[][] = [];
  for (let r = sel.top; r <= sel.bottom; r++) {
    const rowId = input.rowIds[r];
    if (rowId === undefined) continue;
    const line: string[] = [];
    for (let c = sel.left; c <= sel.right; c++) {
      const col = input.columns[c];
      if (!col) continue;
      line.push(toText(rowId, col.id, input.valueAt(rowId, col.id)));
    }
    out.push(line);
  }
  return out;
}

/** Yazdırılabilir tek karakter mi (hücrede yazmaya başlamak için)? */
export function isPrintableKey(e: {
  key: string;
  ctrlKey?: boolean;
  metaKey?: boolean;
  altKey?: boolean;
}): boolean {
  return e.key.length === 1 && !e.ctrlKey && !e.metaKey && !e.altKey;
}

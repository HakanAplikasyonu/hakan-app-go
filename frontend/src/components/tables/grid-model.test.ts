import { describe, expect, it } from "vitest";

import {
  applyChanges,
  clearChanges,
  copyMatrix,
  fillChanges,
  fillDownChanges,
  fillRightChanges,
  fillTarget,
  forgetRow,
  isPrintableKey,
  moveCoord,
  normalizeRange,
  parseLocaleDecimal,
  parseTsv,
  pasteChanges,
  pushHistory,
  redoHistory,
  toTsv,
  undoHistory,
  EMPTY_HISTORY,
  type CellChange,
  type GridModelInput,
} from "./grid-model";

const b = { rows: 3, cols: 4 };

describe("moveCoord", () => {
  it("oklar sınırda durur, Ctrl kenara atlar", () => {
    expect(moveCoord({ row: 0, col: 0 }, "ArrowUp", b)).toEqual({
      row: 0,
      col: 0,
    });
    expect(moveCoord({ row: 1, col: 1 }, "ArrowRight", b)).toEqual({
      row: 1,
      col: 2,
    });
    expect(
      moveCoord({ row: 1, col: 1 }, "ArrowDown", b, { ctrl: true }),
    ).toEqual({ row: 2, col: 1 });
    expect(
      moveCoord({ row: 1, col: 1 }, "ArrowLeft", b, { ctrl: true }),
    ).toEqual({ row: 1, col: 0 });
  });

  it("Tab satır sonunda alt satıra sarar, Shift+Tab geri", () => {
    expect(moveCoord({ row: 0, col: 3 }, "Tab", b)).toEqual({ row: 1, col: 0 });
    expect(moveCoord({ row: 1, col: 0 }, "Tab", b, { shift: true })).toEqual({
      row: 0,
      col: 3,
    });
    // Son hücrede yerinde kalır (ızgaradan çıkış için).
    expect(moveCoord({ row: 2, col: 3 }, "Tab", b)).toEqual({ row: 2, col: 3 });
    expect(moveCoord({ row: 0, col: 0 }, "Tab", b, { shift: true })).toEqual({
      row: 0,
      col: 0,
    });
  });

  it("Enter aşağı, Shift+Enter yukarı; Home/End ve sayfa", () => {
    expect(moveCoord({ row: 0, col: 2 }, "Enter", b)).toEqual({
      row: 1,
      col: 2,
    });
    expect(moveCoord({ row: 1, col: 2 }, "Enter", b, { shift: true })).toEqual({
      row: 0,
      col: 2,
    });
    expect(moveCoord({ row: 1, col: 2 }, "Home", b)).toEqual({
      row: 1,
      col: 0,
    });
    expect(moveCoord({ row: 1, col: 2 }, "End", b, { ctrl: true })).toEqual({
      row: 2,
      col: 3,
    });
    expect(
      moveCoord({ row: 0, col: 0 }, "PageDown", b, { pageSize: 10 }),
    ).toEqual({ row: 2, col: 0 });
  });

  it("sağdan sola yazımda sol/sağ yer değiştirir", () => {
    expect(
      moveCoord({ row: 0, col: 1 }, "ArrowLeft", b, { rtl: true }),
    ).toEqual({ row: 0, col: 2 });
  });
});

describe("parseTsv / toTsv", () => {
  it("Excel panosu: sekme ve satır sonları, sondaki satır sonu", () => {
    expect(parseTsv("a\tb\r\nc\td\r\n")).toEqual([
      ["a", "b"],
      ["c", "d"],
    ]);
    expect(parseTsv("tek")).toEqual([["tek"]]);
    expect(parseTsv("")).toEqual([]);
    expect(parseTsv("a\t\tc")).toEqual([["a", "", "c"]]);
  });

  it("tırnaklı alan: satır sonu, sekme ve kaçış", () => {
    expect(parseTsv('"iki\nsatır"\t"x""y"\n')).toEqual([["iki\nsatır", 'x"y']]);
    expect(parseTsv('a"b\tc')).toEqual([['a"b', "c"]]);
  });

  it("toTsv gerekeni tırnaklar ve gidiş-dönüş korunur", () => {
    const m = [
      ["600.01", 'Satış, "yurt içi"'],
      ["a\tb", ""],
    ];
    expect(toTsv(m)).toBe('600.01\t"Satış, ""yurt içi"""\n"a\tb"\t');
    expect(parseTsv(toTsv(m))).toEqual(m);
  });
});

describe("parseLocaleDecimal", () => {
  it.each([
    ["1.234,56", "1234.56"],
    ["1234,5", "1234.5"],
    ["1,234.56", "1234.56"],
    ["₺ 1.284.560,40", "1284560.4"],
    ["1.500", "1500"],
    ["0.500", "0.5"],
    ["1.5", "1.5"],
    ["-12,50", "-12.5"],
    ["(12,5)", "-12.5"],
    ["100 TL", "100"],
    ["007", "7"],
    ["-0", "0"],
    [",5", "0.5"],
  ])("%s → %s", (input, out) => {
    expect(parseLocaleDecimal(input)).toBe(out);
  });

  it.each(["", "abc", "1.2.3", "1,2,3", "1.234.5,6,7", "12a"])(
    "%s çözülmez",
    (input) => {
      expect(parseLocaleDecimal(input)).toBeNull();
    },
  );
});

/** 3×3 ızgara: kolon c2 salt okunur. */
function makeInput(values: Record<string, Record<string, unknown>>) {
  const rowIds = ["r1", "r2", "r3"];
  const input: GridModelInput = {
    rowIds,
    columns: ["c1", "c2", "c3"].map((id) => ({
      id,
      editable: () => id !== "c2",
      parse: (text: string) => (text === "" ? null : text.toUpperCase()),
    })),
    valueAt: (r, c) => values[r]?.[c] ?? null,
  };
  return input;
}

describe("pasteChanges", () => {
  it("tek hücreden başlayıp sığdığı kadar yazar, salt okunuru atlar", () => {
    const input = makeInput({});
    const changes = pasteChanges(
      input,
      { top: 1, bottom: 1, left: 0, right: 0 },
      [
        ["a", "b", "c", "taşan"],
        ["d", "e", "f"],
        ["g", "h", "i"],
      ],
    );
    expect(changes.map((c) => [c.rowId, c.columnId, c.next])).toEqual([
      ["r2", "c1", "A"],
      ["r2", "c3", "C"],
      ["r3", "c1", "D"],
      ["r3", "c3", "F"],
    ]);
  });

  it("tek değer tüm seçime yayılır; aynı değer değişiklik sayılmaz", () => {
    const input = makeInput({ r1: { c1: "X" } });
    const changes = pasteChanges(
      input,
      { top: 0, bottom: 2, left: 0, right: 0 },
      [["x"]],
    );
    expect(changes.map((c) => c.rowId)).toEqual(["r2", "r3"]);
  });
});

describe("doldurma", () => {
  const input = makeInput({
    r1: { c1: "A", c3: "Z" },
    r2: { c1: "B" },
  });

  it("Ctrl+D ilk satırı aşağı kopyalar", () => {
    const ch = fillDownChanges(input, { top: 0, bottom: 2, left: 0, right: 2 });
    expect(ch.map((c) => [c.rowId, c.columnId, c.prev, c.next])).toEqual([
      ["r2", "c1", "B", "A"],
      ["r2", "c3", null, "Z"],
      ["r3", "c1", null, "A"],
      ["r3", "c3", null, "Z"],
    ]);
  });

  it("Ctrl+R ilk kolonu sağa kopyalar (salt okunur atlanır)", () => {
    const ch = fillRightChanges(input, {
      top: 0,
      bottom: 0,
      left: 0,
      right: 2,
    });
    expect(ch.map((c) => [c.columnId, c.next])).toEqual([["c3", "A"]]);
  });

  it("tutamaç: iki satırlık deseni tekrarlar", () => {
    const source = { top: 0, bottom: 1, left: 0, right: 0 };
    const target = fillTarget(source, { row: 2, col: 1 });
    expect(target).toEqual({ top: 0, bottom: 2, left: 0, right: 0 });
    const ch = fillChanges(input, source, target);
    expect(ch).toEqual([
      { rowId: "r3", columnId: "c1", prev: null, next: "A" },
    ]);
  });

  it("tutamaç yatayda baskınsa sağa genişler, yerinde kalırsa kaynak döner", () => {
    const source = { top: 0, bottom: 0, left: 0, right: 0 };
    expect(fillTarget(source, { row: 1, col: 2 })).toEqual({
      ...source,
      right: 2,
    });
    expect(fillTarget(source, { row: 0, col: 0 })).toEqual(source);
  });
});

describe("temizle / kopyala", () => {
  const input = makeInput({ r1: { c1: "A", c2: "RO", c3: "C" } });
  it("Delete düzenlenebilir hücreleri boşaltır", () => {
    const ch = clearChanges(input, { top: 0, bottom: 0, left: 0, right: 2 });
    expect(ch.map((c) => c.columnId)).toEqual(["c1", "c3"]);
  });
  it("kopyalama salt okunurları da alır", () => {
    const m = copyMatrix(
      input,
      { top: 0, bottom: 1, left: 0, right: 2 },
      (_r, _c, v) => (v == null ? "" : String(v)),
    );
    expect(m).toEqual([
      ["A", "RO", "C"],
      ["", "", ""],
    ]);
  });
});

describe("taslaklar ve geçmiş", () => {
  const original = (r: string, c: string) =>
    r === "r1" && c === "c1" ? "A" : null;
  const ch1: CellChange[] = [
    { rowId: "r1", columnId: "c1", prev: "A", next: "B" },
  ];
  const ch2: CellChange[] = [
    { rowId: "r2", columnId: "c3", prev: null, next: "X" },
  ];

  it("özgün değere dönen hücre ve satır taslaktan düşer", () => {
    const d1 = applyChanges({}, ch1, original);
    expect(d1).toEqual({ r1: { c1: "B" } });
    expect(applyChanges(d1, ch1, original, "backward")).toEqual({});
  });

  it("geri al / yinele sırası; yeni işlem ileriyi siler", () => {
    let h = pushHistory(EMPTY_HISTORY, ch1);
    h = pushHistory(h, ch2);
    const u = undoHistory(h)!;
    expect(u.batch).toBe(ch2);
    const r = redoHistory(u.history)!;
    expect(r.batch).toBe(ch2);
    expect(pushHistory(u.history, ch1).future).toEqual([]);
    expect(undoHistory(EMPTY_HISTORY)).toBeNull();
    expect(pushHistory(h, [])).toBe(h);
  });

  it("geçmiş sınırı ve kaydedilen satırın unutulması", () => {
    let h = EMPTY_HISTORY;
    for (let i = 0; i < 5; i++) h = pushHistory(h, ch1, 3);
    expect(h.past).toHaveLength(3);
    const mixed = pushHistory(EMPTY_HISTORY, [...ch1, ...ch2]);
    expect(forgetRow(mixed, "r1").past).toEqual([ch2]);
    expect(forgetRow(pushHistory(EMPTY_HISTORY, ch1), "r1").past).toEqual([]);
  });
});

describe("yardımcılar", () => {
  it("normalizeRange ters seçimi düzeltir", () => {
    expect(
      normalizeRange({ anchor: { row: 2, col: 3 }, focus: { row: 0, col: 1 } }),
    ).toEqual({ top: 0, bottom: 2, left: 1, right: 3 });
  });
  it("isPrintableKey", () => {
    expect(isPrintableKey({ key: "a" })).toBe(true);
    expect(isPrintableKey({ key: "a", ctrlKey: true })).toBe(false);
    expect(isPrintableKey({ key: "Enter" })).toBe(false);
  });
});

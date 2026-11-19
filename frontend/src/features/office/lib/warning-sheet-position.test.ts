import { describe, expect, it } from "vitest";

import { sheetPosition } from "./warning-sheet-position";

const items = [{ uuid: "a" }, { uuid: "b" }, { uuid: "c" }];

describe("sheetPosition", () => {
  it("finds the open warning and its neighbours", () => {
    const p = sheetPosition(items, "b", null);
    expect(p.current?.uuid).toBe("b");
    expect(p.gone).toBe(false);
    expect(p.index).toBe(1);
    expect(p.prev?.uuid).toBe("a");
    expect(p.next?.uuid).toBe("c");
  });

  it("has no prev on the first and no next on the last", () => {
    expect(sheetPosition(items, "a", null).prev).toBeUndefined();
    expect(sheetPosition(items, "c", null).next).toBeUndefined();
  });

  it("is closed when the warning is neither listed nor last opened", () => {
    const p = sheetPosition(items, "x", null);
    expect(p.current).toBeNull();
  });

  it("keeps a resolved warning and points next at its replacement", () => {
    // "b" (index 1) was resolved and dropped out of the list.
    const after = [{ uuid: "a" }, { uuid: "c" }];
    const p = sheetPosition(after, "b", { warning: { uuid: "b" }, index: 1 });
    expect(p.current?.uuid).toBe("b");
    expect(p.gone).toBe(true);
    expect(p.next?.uuid).toBe("c");
    expect(p.prev?.uuid).toBe("a");
  });

  it("has no next when the last warning dropped out", () => {
    const after = [{ uuid: "a" }];
    const p = sheetPosition(after, "b", { warning: { uuid: "b" }, index: 1 });
    expect(p.gone).toBe(true);
    expect(p.next).toBeUndefined();
    expect(p.prev?.uuid).toBe("a");
  });
});

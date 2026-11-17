import { describe, expect, it } from "vitest";

import {
  applyListSearchParams,
  parseListSearchParams,
  sortingToSortParam,
  sortParamToSorting,
  toHref,
  type ListUrlOptions,
} from "./list-search-params";

const opts: ListUrlOptions = {
  defaultPageSize: 50,
  defaultSort: "-issue_date",
  filterKeys: ["status", "direction"],
};

describe("sıralama parametresi", () => {
  it("gidiş dönüş", () => {
    const sorting = [
      { id: "issue_date", desc: true },
      { id: "name", desc: false },
    ];
    expect(sortingToSortParam(sorting)).toBe("-issue_date,name");
    expect(sortParamToSorting("-issue_date,name")).toEqual(sorting);
    expect(sortingToSortParam([])).toBeUndefined();
    expect(sortParamToSorting(null)).toEqual([]);
  });
});

describe("parseListSearchParams", () => {
  it("boş URL varsayılanları verir", () => {
    const s = parseListSearchParams(new URLSearchParams(), opts);
    expect(s.pagination).toEqual({ pageIndex: 0, pageSize: 50 });
    expect(s.sorting).toEqual([{ id: "issue_date", desc: true }]);
    expect(s.q).toBe("");
    expect(s.columnFilters).toEqual([]);
  });

  it("sayfa 1 tabanlı, süzgeçler virgülle", () => {
    const s = parseListSearchParams(
      new URLSearchParams(
        "page=3&size=20&sort=name&q=acme&status=open,resolved&x=1",
      ),
      opts,
    );
    expect(s.pagination).toEqual({ pageIndex: 2, pageSize: 20 });
    expect(s.sorting).toEqual([{ id: "name", desc: false }]);
    expect(s.q).toBe("acme");
    expect(s.columnFilters).toEqual([
      { id: "status", value: ["open", "resolved"] },
    ]);
  });

  it("bozuk sayılar varsayılana düşer", () => {
    const s = parseListSearchParams(
      new URLSearchParams("page=-2&size=abc"),
      opts,
    );
    expect(s.pagination).toEqual({ pageIndex: 0, pageSize: 50 });
  });

  it("önek", () => {
    const s = parseListSearchParams(new URLSearchParams("a_page=2&page=9"), {
      ...opts,
      prefix: "a_",
    });
    expect(s.pagination.pageIndex).toBe(1);
  });
});

describe("applyListSearchParams", () => {
  it("varsayılanlar yazılmaz", () => {
    const next = applyListSearchParams(
      "",
      {
        pagination: { pageIndex: 0, pageSize: 50 },
        sorting: [{ id: "issue_date", desc: true }],
      },
      opts,
    );
    expect(next.toString()).toBe("");
  });

  it("süzgeç değişince sayfa başa döner, diğer anahtarlar korunur", () => {
    const next = applyListSearchParams(
      "page=4&engagement=e1",
      { columnFilters: [{ id: "status", value: ["open"] }] },
      opts,
    );
    expect(next.get("page")).toBeNull();
    expect(next.get("status")).toBe("open");
    expect(next.get("engagement")).toBe("e1");
  });

  it("arama ve sayfa", () => {
    let next = applyListSearchParams("", { q: "  acme " }, opts);
    expect(next.get("q")).toBe("acme");
    next = applyListSearchParams(
      next,
      { pagination: { pageIndex: 2, pageSize: 20 } },
      opts,
    );
    expect(next.toString()).toBe("q=acme&size=20&page=3");
    next = applyListSearchParams(next, { q: "" }, opts);
    expect(next.get("q")).toBeNull();
    expect(next.get("page")).toBeNull();
  });

  it("ek anahtarlar: boş değer siler", () => {
    const next = applyListSearchParams(
      "invoice=u1&direction=incoming",
      { extra: { invoice: "", tab: "lines" } },
      opts,
    );
    expect(next.get("invoice")).toBeNull();
    expect(next.get("tab")).toBe("lines");
    expect(next.get("direction")).toBe("incoming");
  });

  it("sıralama kaldırılınca anahtar silinir", () => {
    const next = applyListSearchParams("sort=name", { sorting: [] }, opts);
    expect(next.get("sort")).toBeNull();
  });

  it("toHref", () => {
    expect(toHref("/a", new URLSearchParams())).toBe("/a");
    expect(toHref("/a", new URLSearchParams("x=1"))).toBe("/a?x=1");
  });
});

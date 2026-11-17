import { describe, expect, it } from "vitest";

import { carryListParams, tabState } from "./engagement-nav";

const base = "/t/demo/engagements/e1";

describe("tabState", () => {
  it("tam eşleşmede page", () => {
    expect(tabState(`${base}/invoices`, { href: `${base}/invoices` })).toBe(
      "page",
    );
    expect(tabState(`${base}/invoices/`, { href: `${base}/invoices` })).toBe(
      "page",
    );
  });

  it("sekme altındaki detayda true", () => {
    expect(tabState(`${base}/invoices/i9`, { href: `${base}/invoices` })).toBe(
      "true",
    );
  });

  it("genel bakış yalnızca tam eşleşmede", () => {
    const overview = { href: base, exact: true };
    expect(tabState(base, overview)).toBe("page");
    expect(tabState(`${base}/invoices`, overview)).toBeUndefined();
  });

  it("önek benzerliği eşleşme sayılmaz", () => {
    expect(
      tabState(`${base}/invoices-x`, { href: `${base}/invoices` }),
    ).toBeUndefined();
  });

  it("sorgu ve kodlama yok sayılır", () => {
    expect(
      tabState("/t/%C3%B6/engagements/e1/bank", {
        href: "/t/ö/engagements/e1/bank?view=rules",
      }),
    ).toBe("page");
  });
});

describe("carryListParams", () => {
  it("süzgeç ve sayfa taşınır, çekmece ve mükellef düşer", () => {
    const p = new URLSearchParams(
      "direction=incoming&page=2&invoice=x&taxpayer=e1",
    );
    expect(carryListParams(p)).toBe("?direction=incoming&page=2");
  });

  it("boşsa soru işareti yok", () => {
    expect(carryListParams(new URLSearchParams("invoice=x"))).toBe("");
  });
});

import { describe, expect, it } from "vitest";

import { actionHref } from "./actions";

describe("actionHref", () => {
  it("genel ekranda ofis düzeyi sayfa, mükellef süzgeciyle", () => {
    expect(actionHref("demo", "open_review", "e1")).toBe(
      "/t/demo/review?taxpayer=e1",
    );
    expect(actionHref("demo", "open_journals", "e1")).toBe(
      "/t/demo/journals?taxpayer=e1",
    );
  });

  it("mükellef detayında mükellef sekmesi (#468)", () => {
    expect(actionHref("demo", "open_review", "e1", true)).toBe(
      "/t/demo/engagements/e1/review",
    );
    expect(actionHref("demo", "open_journals", "e1", true)).toBe(
      "/t/demo/engagements/e1/journals",
    );
    expect(actionHref("demo", "bank_suggest", "e1", true)).toBe(
      "/t/demo/engagements/e1/bank",
    );
  });

  it("eksik belge açık belge isteklerine gider (#563)", () => {
    expect(actionHref("demo", "open_documents", "e1")).toBe(
      "/t/demo/engagements/e1#belge-istekleri",
    );
    expect(actionHref("demo", "open_documents", "e1", true)).toBe(
      "/t/demo/engagements/e1#belge-istekleri",
    );
  });
});

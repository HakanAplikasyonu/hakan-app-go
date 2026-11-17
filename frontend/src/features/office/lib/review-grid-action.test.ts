import { describe, expect, it } from "vitest";

import { gridRowAction } from "./review-grid-action";

describe("gridRowAction", () => {
  const base = { proposedCode: "770.01", code: "770.01", decision: null };

  it("değişmeyen kod ve kararsız satır kaydedilmez", () => {
    expect(gridRowAction(base)).toBeNull();
  });
  it("Onayla önerilen kodu onaylar", () => {
    expect(gridRowAction({ ...base, decision: "accept" })).toBe("accept");
  });
  it("farklı kod düzeltmedir (Onayla seçili olsa da)", () => {
    expect(gridRowAction({ ...base, code: "760.01" })).toBe("correct");
    expect(
      gridRowAction({ ...base, code: " 760.01 ", decision: "accept" }),
    ).toBe("correct");
    expect(gridRowAction({ ...base, proposedCode: undefined })).toBe("correct");
  });
  it("Reddet her zaman ret", () => {
    expect(gridRowAction({ ...base, code: "760.01", decision: "reject" })).toBe(
      "reject",
    );
  });
  it("boş kod düzeltme sayılmaz", () => {
    expect(gridRowAction({ ...base, code: "" })).toBeNull();
  });
});

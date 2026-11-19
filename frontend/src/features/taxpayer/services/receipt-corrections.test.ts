import { describe, expect, it } from "vitest";

import { buildCorrections } from "./taxpayer.service";

describe("buildCorrections", () => {
  it("keeps filled fields and adds the note", () => {
    expect(
      buildCorrections(
        {
          vendor_name: " Akaryakıt A.Ş. ",
          total_amount: "",
          date: "2026-11-02",
        },
        " plaka yanlış ",
      ),
    ).toEqual([
      { field: "vendor_name", value: "Akaryakıt A.Ş.", note: "plaka yanlış" },
      { field: "date", value: "2026-11-02", note: "plaka yanlış" },
    ]);
  });

  it("leaves the note out when empty", () => {
    expect(buildCorrections({ tax_amount: "18" }, "")).toEqual([
      { field: "tax_amount", value: "18" },
    ]);
  });
});

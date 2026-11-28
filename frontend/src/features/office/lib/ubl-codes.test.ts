import { describe, expect, it } from "vitest";

import tr from "@/locales/tr/office.json";

import { invoiceKindText, invoiceTypeLabel } from "./ubl-codes";

const t = (key: string) =>
  (tr as Record<string, string>)[key.replace(/^office\./, "")] ?? key;

describe("UBL kodları Türkçe (#455)", () => {
  it("tür ve senaryo", () => {
    expect(invoiceKindText("SATIS", "TICARIFATURA", t)).toBe(
      "Satış · Ticari e-Fatura",
    );
    expect(invoiceKindText("TEVKIFAT", null, t)).toBe("Tevkifatlı");
    expect(invoiceKindText("IADE", "EARSIVFATURA", t)).toBe(
      "İade · e-Arşiv fatura",
    );
  });
  it("bilinmeyen kod aynen kalır, boş değer boş", () => {
    expect(invoiceTypeLabel("YENIKOD", t)).toBe("YENIKOD");
    expect(invoiceKindText(null, undefined, t)).toBe("");
  });
});

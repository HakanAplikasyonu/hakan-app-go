import { describe, expect, it, vi } from "vitest";

vi.mock("../hooks/use-office-invoices", () => ({}));

const { nextPeriod, vatDeductionRange } =
  await import("./vat-deduction-period");

describe("KDV indirim dönemi aralığı (#640)", () => {
  it("bir sonraki ay", () => {
    expect(nextPeriod("2026-03")).toBe("2026-04");
    expect(nextPeriod("2026-12")).toBe("2027-01");
  });
  it("belge ayından sonra, en geç izleyen yılın Aralık'ı", () => {
    expect(vatDeductionRange("2026-10")).toEqual({
      min: "2026-11",
      max: "2027-12",
    });
    expect(vatDeductionRange("2026-12")).toEqual({
      min: "2027-01",
      max: "2027-12",
    });
  });
});

import { describe, expect, it } from "vitest";

import { formatAmount, formatMoney, parseAmountInput } from "./decimal";

describe("parseAmountInput", () => {
  it.each([
    ["", ""],
    ["1250", "1250.00"],
    ["1250,5", "1250.50"],
    ["1.250,50", "1250.50"],
    ["1.250", "1250.00"],
    ["1.250.000", "1250000.00"],
    ["12.5", "12.50"],
    ["12.50", "12.50"],
    ["₺ 3.000,00", "3000.00"],
    [",5", "0.50"],
  ])("%s → %s", (input, want) => {
    expect(parseAmountInput(input)).toBe(want);
  });

  it.each(["abc", "1,2,3", "12.34.5", "1,234", "1.2345", "-5", "12,345"])(
    "%s geçersiz",
    (input) => {
      expect(parseAmountInput(input)).toBeNull();
    },
  );
});

describe("formatAmount", () => {
  it("biçimler", () => {
    expect(formatAmount("1284560.40")).toBe("1.284.560,40");
    expect(formatAmount("-8.5")).toBe("-8,50");
    expect(formatAmount(null)).toBe("0,00");
    expect(formatAmount(12.5)).toBe("12,50");
  });

  it.each(["1,5", "NaN", "abc", "1.234,56", "1e5"])(
    "ondalık olmayan değerde (%s) düşmez, olduğu gibi döner",
    (v) => {
      expect(() => formatAmount(v)).not.toThrow();
      expect(formatAmount(v)).toBe(v);
      expect(() => formatMoney(v, "TRY")).not.toThrow();
    },
  );
});

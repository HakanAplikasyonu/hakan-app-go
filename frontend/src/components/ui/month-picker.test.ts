import { describe, expect, it } from "vitest";

import { formatMonthValue, parseMonthValue } from "./month-picker";

describe("month-picker value helpers", () => {
  it("parses yyyy-MM", () => {
    expect(parseMonthValue("2026-11")).toEqual({ year: 2026, month: 10 });
    expect(parseMonthValue("2026-01")).toEqual({ year: 2026, month: 0 });
  });

  it("rejects empty and malformed values", () => {
    expect(parseMonthValue(undefined)).toBeUndefined();
    expect(parseMonthValue("")).toBeUndefined();
    expect(parseMonthValue("2026-13")).toBeUndefined();
    expect(parseMonthValue("2026-00")).toBeUndefined();
    expect(parseMonthValue("2026-11-19")).toBeUndefined();
  });

  it("formats with zero padding and round-trips", () => {
    expect(formatMonthValue(2026, 0)).toBe("2026-01");
    expect(parseMonthValue(formatMonthValue(2027, 11))).toEqual({
      year: 2027,
      month: 11,
    });
  });
});

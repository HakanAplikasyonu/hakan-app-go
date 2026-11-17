import { describe, expect, it } from "vitest";

import { periodRange } from "./use-easytrade";

describe("periodRange", () => {
  it("covers a closed month", () => {
    expect(periodRange("2026-10", new Date(2026, 10, 17))).toEqual({
      from: "2026-10-01",
      to: "2026-10-31",
    });
  });
  it("stops at today in the current month", () => {
    expect(periodRange("2026-11", new Date(2026, 10, 17))).toEqual({
      from: "2026-11-01",
      to: "2026-11-17",
    });
  });
  it("knows February", () => {
    expect(periodRange("2028-02", new Date(2028, 5, 1)).to).toBe("2028-02-29");
  });
});

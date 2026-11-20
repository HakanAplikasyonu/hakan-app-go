import { describe, expect, it } from "vitest";

import { daysLeft } from "./office-tax-returns-page";

describe("daysLeft", () => {
  const today = new Date(2026, 10, 20, 15, 30); // 20 Kasım 2026, öğleden sonra
  it("son güne kalan gün", () => {
    expect(daysLeft("2026-11-30", today)).toBe(10);
    expect(daysLeft("2026-11-20", today)).toBe(0);
    expect(daysLeft("2026-11-18", today)).toBe(-2);
  });
});

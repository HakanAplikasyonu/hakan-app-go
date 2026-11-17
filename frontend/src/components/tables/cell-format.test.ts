import { describe, expect, it } from "vitest";

import {
  compareDate,
  compareDecimal,
  formatCellDate,
  formatCellMoney,
  intlLocale,
} from "./cell-format";

describe("formatCellMoney", () => {
  it("tr-TR gruplama, her zaman iki hane, para simgesi", () => {
    expect(formatCellMoney("1284560.4")).toBe("₺ 1.284.560,40");
    expect(formatCellMoney("0")).toBe("₺ 0,00");
    expect(formatCellMoney("-12.345", "EUR")).toBe("€ -12,35");
  });

  it("simgesi olmayan para birimi koduyla yazılır", () => {
    expect(formatCellMoney("10", "CHF")).toBe("10,00 CHF");
  });

  it("boş değer tire, sayı olmayan değer ham döner", () => {
    expect(formatCellMoney(undefined)).toBe("—");
    expect(formatCellMoney("")).toBe("—");
    expect(formatCellMoney("abc")).toBe("abc");
  });

  it("number girdi kuruşa yuvarlanır", () => {
    expect(formatCellMoney(1500)).toBe("₺ 1.500,00");
  });
});

describe("formatCellDate", () => {
  it("yalnızca gün olan değeri kaydırmadan biçimler (tr-TR)", () => {
    expect(formatCellDate("2026-11-17", "tr")).toBe("17.11.2026");
  });

  it("en için en-GB", () => {
    expect(formatCellDate("2026-11-17", "en")).toBe("17/11/2026");
  });

  it("medium ve datetime biçimleri", () => {
    expect(formatCellDate("2026-11-17", "tr", "medium")).toMatch(/17 Kas 2026/);
    expect(formatCellDate("2026-11-17T09:05:00+03:00", "tr", "datetime")).toBe(
      "17.11.2026 09:05",
    );
  });

  it("boş ve geçersiz değerler", () => {
    expect(formatCellDate(null, "tr")).toBe("—");
    expect(formatCellDate("", "tr", "short", "")).toBe("");
    expect(formatCellDate("yarın", "tr")).toBe("yarın");
  });
});

describe("karşılaştırıcılar", () => {
  it("ondalık string sayısal sıralanır", () => {
    const values = ["100.00", "9.50", "-1", "1000"];
    expect([...values].sort(compareDecimal)).toEqual([
      "-1",
      "9.50",
      "100.00",
      "1000",
    ]);
  });

  it("tarih sıralaması, boşlar sonda", () => {
    const values = ["2026-03-01", "", "2025-12-31", "2026-01-15"];
    expect([...values].sort(compareDate)).toEqual([
      "2025-12-31",
      "2026-01-15",
      "2026-03-01",
      "",
    ]);
  });

  it("intlLocale", () => {
    expect(intlLocale("tr")).toBe("tr-TR");
    expect(intlLocale("en")).toBe("en-GB");
  });
});

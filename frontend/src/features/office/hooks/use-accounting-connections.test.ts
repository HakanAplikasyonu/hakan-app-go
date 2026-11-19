import { beforeEach, describe, expect, it, vi } from "vitest";

import {
  accountingStep,
  rememberParasutReturn,
  reportTotals,
  takeParasutReturn,
} from "./use-accounting-connections";

describe("accountingStep", () => {
  it("follows consent, company and status", () => {
    expect(accountingStep({ setup_state: "draft", status: "pending" })).toBe(
      "authorize",
    );
    expect(
      accountingStep({ setup_state: "credentials_entered", status: "pending" }),
    ).toBe("company");
    expect(
      accountingStep({
        setup_state: "active",
        status: "active",
        company_id: "1",
      }),
    ).toBe("active");
    expect(
      accountingStep({
        setup_state: "active",
        status: "auth_failed",
        company_id: "1",
      }),
    ).toBe("failed");
  });
});

describe("reportTotals", () => {
  it("sums written expenses and excluded documents", () => {
    expect(
      reportTotals({
        invoices_created: 3,
        invoices_existing: 1,
        expenses_created: 2,
        expenses_updated: 1,
        expenses_unchanged: 5,
        possible_duplicates: 1,
        excluded: { cancelled: 2, not_an_invoice: 1 },
      }),
    ).toEqual({ invoices: 3, expenses: 3, duplicates: 1, excluded: 3 });
    expect(reportTotals(undefined)).toBeNull();
  });
});

describe("Paraşüt return", () => {
  beforeEach(() => {
    const m = new Map<string, string>();
    vi.stubGlobal("sessionStorage", {
      getItem: (k: string) => m.get(k) ?? null,
      setItem: (k: string, v: string) => void m.set(k, v),
      removeItem: (k: string) => void m.delete(k),
    });
  });
  const v = { slug: "ofis", engagement: "e1", connection: "c1" };

  it("is read once", () => {
    rememberParasutReturn(v);
    expect(takeParasutReturn("c1")).toEqual(v);
    expect(takeParasutReturn("c1")).toBeNull();
  });

  it("refuses another connection's state", () => {
    rememberParasutReturn(v);
    expect(takeParasutReturn("c2")).toBeNull();
  });
});

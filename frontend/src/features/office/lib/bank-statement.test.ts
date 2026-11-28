import { describe, expect, it } from "vitest";

import type {
  BankStatementAnalysis,
  BankStatementReport,
} from "../services/bank-posting.service";
import { accountsOf, periodsOf, safeAnalysis } from "./bank-statement";

const account = (
  over: Partial<BankStatementReport["accounts"][number]>,
): BankStatementReport["accounts"][number] =>
  ({
    account: "",
    cash_box_uuid: "00000000-0000-0000-0000-000000000000",
    lines: 1,
    inserted: 1,
    merged: 0,
    skipped: 0,
    possible_duplicates: 0,
    total_credit: "0",
    total_debit: "0",
    ...over,
  }) as BankStatementReport["accounts"][number];

describe("accountsOf / periodsOf", () => {
  it("accounts null gelse de boş dizi döner", () => {
    const rep = { accounts: null } as unknown as BankStatementReport;
    expect(accountsOf(rep)).toEqual([]);
    expect(periodsOf(rep)).toEqual([]);
    expect(periodsOf(undefined)).toEqual([]);
  });

  it("yeni hareketli ayları sayar, yıl dönümünü geçer", () => {
    const rep = {
      accounts: [
        account({ from: "2026-11-03", to: "2027-01-20" }),
        account({ from: "2026-05-01", to: "2026-05-31", inserted: 0 }),
      ],
    } as BankStatementReport;
    expect(periodsOf(rep)).toEqual(["2026-11", "2026-12", "2027-01"]);
  });

  it("geçersiz tarihli hesabı atlar (sonsuz döngü ya da NaN yok)", () => {
    const rep = {
      accounts: [
        account({ from: "31.10.2026", to: "2026-10-31" }),
        account({ from: "", to: "" }),
        account({ from: "2026-13-01", to: "2026-13-02" }),
      ],
    } as BankStatementReport;
    expect(periodsOf(rep)).toEqual([]);
  });
});

describe("safeAnalysis", () => {
  it("null listeleri, eksik hücreleri ve çift satırı düzeltir", () => {
    const raw = {
      format: "excel",
      header_row: 1,
      headers: null,
      fingerprint: "x",
      columns: [
        { index: 0, header: "Tarih", field: "date" },
        null,
        { index: 2, header: null, field: "" },
      ],
      sample: null,
      lines: 0,
      row_errors: 0,
      warnings: null,
      rows: [
        { row: 2, cells: null, kind: "movement" },
        { row: 2, cells: ["x"], kind: "movement" },
        { row: 3, cells: ["01.10.2026", null], kind: "error" },
        null,
      ],
    } as unknown as BankStatementAnalysis;
    const a = safeAnalysis(raw);
    expect(a.columns.map((c) => c.header)).toEqual(["Tarih", ""]);
    expect(a.warnings).toEqual([]);
    expect(a.rows.map((r) => r.row)).toEqual([2, 3]);
    expect(a.rows[0].cells).toEqual([]);
    expect(a.rows[1].cells).toEqual(["01.10.2026", ""]);
  });
});

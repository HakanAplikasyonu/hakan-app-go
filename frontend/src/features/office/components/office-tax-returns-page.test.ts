import { describe, expect, it } from "vitest";

import { daysLeft, taxReturnsSheet } from "./office-tax-returns-page";

describe("daysLeft", () => {
  const today = new Date(2026, 10, 20, 15, 30); // 20 Kasım 2026, öğleden sonra
  it("son güne kalan gün", () => {
    expect(daysLeft("2026-11-30", today)).toBe(10);
    expect(daysLeft("2026-11-20", today)).toBe(0);
    expect(daysLeft("2026-11-18", today)).toBe(-2);
  });
});

describe("taxReturnsSheet", () => {
  const t = (key: string, vars?: Record<string, string | number>) =>
    vars ? `${key}(${Object.values(vars).join(",")})` : key;
  const row = {
    engagement_uuid: "e1",
    taxpayer_title: "Atlas Gıda",
    taxpayer_tax_id: "9000000017",
    cells: [],
    cell: {
      period: "2026-10",
      returns: [
        {
          kind: "kdv1",
          status: "filed",
          payable_amount: "1234.50",
          filed_on: "2026-11-26",
        },
        {
          kind: "muhsgk",
          status: "draft",
          payable_amount: "99.00",
          filed_on: null,
        },
      ],
      open_blockers: 2,
      open_document_tasks: 1,
      submitted_documents: 0,
      missing_bank_statements: ["Ziraat TL"],
      pending: { bank_transactions: 9, expenses: 0, retail_sales: 8 },
    },
  } as unknown as Parameters<typeof taxReturnsSheet>[0][number];

  it("başlık ve satır: durum, ödenecek (sayı), belgeler, engeller", () => {
    const [head, line] = taxReturnsSheet([row], t, ["muhsgk", "babs"]);
    expect(head).toHaveLength(2 + 3 * 2 + 2);
    expect(line).toEqual([
      "Atlas Gıda",
      "9000000017",
      "office.tax_returns.status_filed",
      1234.5,
      "office.tax_returns.status_draft (office.tax_returns.office.late)",
      "",
      "office.tax_returns.office.not_started (office.tax_returns.office.late)",
      "",
      "office.tax_returns.office.statement_missing(Ziraat TL); office.tax_returns.office.documents_open(1)",
      "office.tax_returns.office.blockers(2); office.tax_returns.office.pending_bank(9); office.tax_returns.office.pending_retail(8)",
    ]);
  });
});

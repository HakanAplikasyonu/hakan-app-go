import { describe, expect, it } from "vitest";

import {
  currentMonth,
  monthOfDate,
  suggestPeriod,
  ublIssueDate,
} from "./document-upload-period";

describe("evrak yükleme dönem önerisi", () => {
  it("belge tarihinin ayını önerir", () => {
    expect(suggestPeriod("2026-10-31")).toEqual({
      period: "2026-10",
      source: "document_date",
    });
  });

  it("tarih yoksa Türkiye saatiyle içinde bulunulan ayı önerir", () => {
    // 30 Kasım 22:30 UTC = İstanbul'da 1 Aralık.
    const now = new Date(Date.UTC(2026, 10, 30, 22, 30));
    expect(currentMonth(now)).toBe("2026-12");
    expect(suggestPeriod("", now)).toEqual({
      period: "2026-12",
      source: "current_month",
    });
  });

  it("geçersiz tarihi yok sayar", () => {
    expect(monthOfDate("31.10.2026")).toBeNull();
    expect(monthOfDate("2026-13-01")).toBeNull();
  });

  it("e-Fatura XML'inden düzenleme tarihini okur", () => {
    const xml =
      '<?xml version="1.0"?><Invoice><cbc:ID>ABC2026</cbc:ID><cbc:IssueDate>2026-09-14</cbc:IssueDate></Invoice>';
    expect(ublIssueDate(xml)).toBe("2026-09-14");
    expect(ublIssueDate("<Invoice/>")).toBeNull();
  });
});

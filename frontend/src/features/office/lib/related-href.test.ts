import { describe, expect, it } from "vitest";

import { routes } from "@/config/routes";

import {
  checklistHref,
  parseFocus,
  reconciliationItemHref,
  relatedHref,
  warningRecordHref,
} from "./related-href";

const U = "0b5e2c1a-8f0e-4c3b-9d2a-1e2f3a4b5c6d";
const base = "/t/demo/engagements/e1";

describe("relatedHref", () => {
  it("fatura kendi detayına, diğerleri sekmede seçili", () => {
    expect(
      relatedHref("demo", "e1", { kind: "invoice", uuid: U, label: "" }),
    ).toBe(`${base}/invoices/${U}`);
    expect(
      relatedHref("demo", "e1", {
        kind: "bank_transaction",
        uuid: U,
        label: "",
      }),
    ).toBe(`${base}/bank?kayit=bank_transaction%3A${U}`);
    expect(
      relatedHref("demo", "e1", { kind: "journal_draft", uuid: U, label: "" }),
    ).toBe(`${base}/journals?kayit=journal_draft%3A${U}`);
  });

  it("kural Uyarılar sekmesinde koda süzülür; cari hesabın bağlantısı yok", () => {
    expect(
      relatedHref("demo", "e1", { kind: "rule", uuid: null, label: "VAT_X" }),
    ).toBe(`${base}/warnings?code=VAT_X`);
    expect(
      relatedHref("demo", "e1", {
        kind: "account",
        uuid: null,
        label: "120.01",
      }),
    ).toBeNull();
  });
});

describe("parseFocus", () => {
  it("geçerli değer", () => {
    expect(parseFocus(`warning:${U}`)).toEqual({ kind: "warning", uuid: U });
  });
  it("bilinmeyen tür ya da bozuk uuid", () => {
    expect(parseFocus(`rule:${U}`)).toBeNull();
    expect(parseFocus("invoice:x")).toBeNull();
    expect(parseFocus(null)).toBeNull();
  });
});

describe("reconciliationItemHref", () => {
  it("fark kalemi kaynağına; elle girilen kalemin bağlantısı yok", () => {
    expect(
      reconciliationItemHref("demo", "e1", { kind: "bank", id: U, label: "" }),
    ).toBe(`${base}/bank?kayit=bank_transaction%3A${U}`);
    expect(
      reconciliationItemHref("demo", "e1", {
        kind: "zreport",
        id: U,
        label: "",
      }),
    ).toBe(`${base}/z-reports?kayit=retail_sale%3A${U}`);
    expect(
      reconciliationItemHref("demo", "e1", {
        kind: "manual",
        id: U,
        label: "",
      }),
    ).toBeNull();
  });
});

describe("checklistHref", () => {
  it("madde kodu ilgili sekmeye", () => {
    expect(checklistHref("demo", "e1", "DRAFT_INVOICES")).toBe(
      `${base}/invoices`,
    );
    expect(checklistHref("demo", "e1", "OPEN_DOCUMENT_TASKS")).toBe(
      routes.tenant.engagementDocTasks("demo", "e1"),
    );
    expect(checklistHref("demo", "e1", "MISSING_COUNTERPARTY_VKN")).toBe(
      `${base}/invoices`,
    );
    expect(checklistHref("demo", "e1", "PENDING_EXPENSES")).toBe(
      `${base}/expenses`,
    );
    expect(checklistHref("demo", "e1", "PENDING_Z_REPORTS")).toBe(
      `${base}/z-reports`,
    );
    expect(checklistHref("demo", "e1", "SGK_ACCRUAL_MISMATCH")).toBe(
      `${base}/payroll`,
    );
    expect(checklistHref("demo", "e1", "NO_DATA")).toBeNull();
  });
});

describe("warningRecordHref", () => {
  const w = (entity_type: string, entity_uuid: string | null = U) => ({
    engagement_uuid: "e1",
    entity_type,
    entity_uuid,
  });
  it("her kayıt türü kendi yerine açılır (#528)", () => {
    expect(warningRecordHref("demo", w("bank_transaction"))).toBe(
      `${base}/bank?kayit=bank_transaction%3A${U}`,
    );
    expect(warningRecordHref("demo", w("invoice_item"))).toBe(
      `${base}/invoices/${U}`,
    );
    expect(warningRecordHref("demo", w("receipt_capture"))).toMatch(
      /\/engagements\/e1\//,
    );
  });
  it("kaydı olmayan ya da bilinmeyen türde bağlantı yok", () => {
    expect(warningRecordHref("demo", w("period", null))).toBeNull();
    expect(warningRecordHref("demo", w("unknown"))).toBeNull();
  });
});

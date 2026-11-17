import { describe, expect, it } from "vitest";

import { parseFocus, relatedHref } from "./related-href";

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

import { describe, expect, it } from "vitest";

import { readDiff } from "./use-change-requests";

describe("readDiff", () => {
  it("reads source and changed fields", () => {
    expect(
      readDiff({
        kind: "update",
        source: { system: "parasut", capture_method: "api" },
        fields: {
          total: { old: "100.00", new: "120.00" },
          note: { old: null, new: "x" },
        },
      }),
    ).toEqual({
      source: "parasut",
      method: "api",
      fields: [
        { field: "note", old: "—", new: "x" },
        { field: "total", old: "100.00", new: "120.00" },
      ],
      // Özet yoksa (eski talep) alanlardan; yön / no / tutar değişmediyse boş.
      summary: {
        number: "",
        direction: "",
        counterparty: "",
        payable: "",
        currency: "TRY",
      },
    });
  });

  it("reads the incoming invoice summary (#710)", () => {
    expect(
      readDiff({
        kind: "create",
        fields: { payable_amount: { new: "492.00" } },
        summary: {
          invoice_number: "GEC2026000000128",
          direction: "outgoing",
          counterparty: "Ali Yılmaz",
          payable_amount: "492.00",
          currency: "TRY",
        },
      }).summary,
    ).toEqual({
      number: "GEC2026000000128",
      direction: "outgoing",
      counterparty: "Ali Yılmaz",
      payable: "492.00",
      currency: "TRY",
    });
  });

  it("survives an unknown shape", () => {
    expect(readDiff(undefined)).toEqual({
      source: "",
      method: "",
      fields: [],
      summary: {
        number: "",
        direction: "",
        counterparty: "",
        payable: "",
        currency: "TRY",
      },
    });
  });
});

import { describe, expect, it } from "vitest";

import {
  kindTitle,
  matchSummary,
  suggestionTitle,
  valueText,
} from "./suggestion-label";

const t = (key: string, vars?: Record<string, string | number>) =>
  ({
    "office.review_api.kind_bank_account": "Banka hesap kodu",
    "office.review_api.counterparty_account": `Cari: ${vars?.name ?? ""}`,
  })[key] ?? key;

describe("bank_account suggestions (#383)", () => {
  it("names the kind", () => {
    expect(kindTitle("bank_account", t)).toBe("Banka hesap kodu");
  });
  it("titles by counterparty, else by the movement's description", () => {
    const base = {
      kind: "bank_account",
      target_type: "bank_transaction",
      target_field: "account_code",
      proposed_value: {
        account_code: "120.01.015",
        account_name: "Oğuz Yılmaz",
      },
    };
    expect(
      suggestionTitle(
        { ...base, provenance: { inputs: { counterparty_name: "X Yazılım" } } },
        t,
      ),
    ).toBe("Cari: X Yazılım");
    expect(
      suggestionTitle(
        { ...base, provenance: { inputs: { description: "EFT UCRETI" } } },
        t,
      ),
    ).toBe("EFT UCRETI");
    expect(valueText(base.proposed_value)).toBe("120.01.015");
  });
});

describe("reconciliation suggestions (#455)", () => {
  const s = {
    kind: "reconciliation",
    target_type: "reconciliation_item",
    target_field: "status",
    proposed_value: { status: "resolved", note: "…" },
    provenance: {
      inputs: {
        main_code: "102",
        ledger_net: "68050.00",
        source_net: "0.00",
        difference: "68050.00",
      },
    },
  };
  it("never titles by the raw field name", () => {
    expect(suggestionTitle(s, t)).toBe("Mutabakat farkı: 102");
    expect(suggestionTitle(s, t)).not.toMatch(/status/i);
  });
  it("summarises amounts in tr-TR", () => {
    expect(matchSummary(s)).toBe(
      "Mizan 68.050,00 TL · Kaynaklar 0,00 TL · Fark 68.050,00 TL",
    );
  });
});

import { describe, expect, it } from "vitest";

import { differenceMeaning } from "./reconciliation-text";

const row = (
  result: string,
  ledger_net: string,
  source_net: string,
  difference: string,
) => ({ result, ledger_net, source_net, difference });

describe("differenceMeaning (#710)", () => {
  it("matched needs no amount", () => {
    expect(
      differenceMeaning(row("matched", "10.00", "10.00", "0.00")).key,
    ).toBe("office.reconciliation.meaning_matched");
  });

  it("positive difference: ledger debit side is larger", () => {
    const m = differenceMeaning(
      row("difference", "1500.00", "1000.00", "500.00"),
    );
    expect(m.key).toBe("office.reconciliation.meaning_ledger_debit");
    expect(m.vars.amount).toBe("₺ 500,00");
  });

  it("negative difference: ledger credit side is larger, amount unsigned", () => {
    const m = differenceMeaning(
      row("difference", "-1500.00", "-1000.00", "-500.00"),
    );
    expect(m.key).toBe("office.reconciliation.meaning_ledger_credit");
    expect(m.vars.amount).toBe("₺ 500,00");
  });

  it("missing on one side names the other side's amount", () => {
    expect(
      differenceMeaning(row("missing_in_ledger", "0.00", "-250.00", "250.00"))
        .vars.amount,
    ).toBe("₺ 250,00");
    expect(
      differenceMeaning(row("missing_in_sources", "75.50", "0.00", "75.50"))
        .vars.amount,
    ).toBe("₺ 75,50");
  });
});

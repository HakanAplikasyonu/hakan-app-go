import { describe, expect, it, vi } from "vitest";

vi.mock("./warnings-page", () => ({}));
vi.mock("./related-records", () => ({}));
vi.mock("./bank-transaction-matching", () => ({}));
vi.mock("./warning-actions", () => ({}));

const { groupWarnings } = await import("./tax-return-warnings");

type W = Parameters<typeof groupWarnings>[0][number];
const w = (
  uuid: string,
  entity_type: string,
  entity_uuid: string | null,
  severity: string,
) => ({ uuid, entity_type, entity_uuid, severity }) as unknown as W;

describe("groupWarnings (#528)", () => {
  it("groups by record, invoice lines under their invoice, period last", () => {
    const g = groupWarnings([
      w("1", "period", null, "blocker"),
      w("2", "invoice_item", "inv1", "warning"),
      w("3", "invoice", "inv1", "warning"),
      w("4", "bank_transaction", "bt1", "blocker"),
    ]);
    expect(g.map((x) => x.key)).toEqual([
      "bank_transaction:bt1",
      "invoice:inv1",
      "period",
    ]);
    expect(g[1]?.items.map((x) => x.uuid)).toEqual(["2", "3"]);
    expect(g[0]?.kind).toBe("bank_transaction");
    expect(g[2]?.kind).toBeNull();
  });
  it("keeps receipts as their own record without a panel kind", () => {
    const [g] = groupWarnings([w("1", "receipt_capture", "rc1", "info")]);
    expect(g?.key).toBe("receipt_capture:rc1");
    expect(g?.kind).toBeNull();
  });
});

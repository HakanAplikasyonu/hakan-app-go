import { describe, expect, it, vi } from "vitest";

import type { ReviewSuggestion } from "../services/review.service";

vi.mock("./warning-actions", () => ({
  WARNINGS_WRITE_PERMISSION: "tenant.warnings.write",
}));
vi.mock("../hooks/use-review", () => ({}));
vi.mock("../hooks/use-warnings", () => ({}));

const { itemSuggestionFor } = await import("./warning-code-fix");

const INV = "11111111-1111-4111-8111-111111111111";

function sug(
  over: Partial<ReviewSuggestion>,
  inputs: Record<string, unknown> = {},
): ReviewSuggestion {
  return {
    uuid: "s",
    kind: "account_code",
    target_type: "invoice_item",
    target_id: 1,
    target_field: "account_code",
    period: "2026-10",
    proposed_value: { account_code: "770.01" },
    confidence: "0.5",
    rationale: "",
    provenance: {
      origin: "rule",
      inputs: { invoice_uuid: INV, item_name: "Dizüstü bilgisayar", ...inputs },
    },
    status: "pending",
    applied_value: null,
    previous_value: null,
    rule_candidate: false,
    decided_at: null,
    created_at: "",
    updated_at: "",
    ...over,
  };
}

describe("itemSuggestionFor (#570)", () => {
  const warning = { entity_uuid: INV, params: { name: "Dizüstü bilgisayar" } };

  it("aynı faturanın aynı kalemindeki bekleyen gider hesabı önerisini bulur", () => {
    const vat = sug({ uuid: "vat", target_field: "vat_account_code" });
    const other = sug({ uuid: "other" }, { item_name: "Kağıt" });
    const line = sug({ uuid: "line" });
    expect(itemSuggestionFor(warning, [vat, other, line])?.uuid).toBe("line");
  });

  it("karar verilmiş, başka faturanın ya da adsız uyarının önerisi yok", () => {
    expect(
      itemSuggestionFor(warning, [sug({ status: "accepted" })]),
    ).toBeNull();
    expect(
      itemSuggestionFor(warning, [sug({}, { invoice_uuid: "x" })]),
    ).toBeNull();
    expect(
      itemSuggestionFor({ entity_uuid: INV, params: {} }, [sug({})]),
    ).toBeNull();
    expect(
      itemSuggestionFor({ entity_uuid: null, params: warning.params }, [
        sug({}),
      ]),
    ).toBeNull();
  });
});

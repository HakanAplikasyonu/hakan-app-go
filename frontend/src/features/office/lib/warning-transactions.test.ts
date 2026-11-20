import { describe, expect, it } from "vitest";

import { warningTransactions } from "./warning-transactions";

const a = "11111111-1111-4111-8111-111111111111";
const b = "22222222-2222-4222-8222-222222222222";
const c = "33333333-3333-4333-8333-333333333333";

describe("warningTransactions (#570)", () => {
  it("reads the movements of counterparty groups", () => {
    expect(
      warningTransactions({
        total: "7900.00",
        groups: [
          { counterparty: "Geç Tedarikçi", transactions: [a, b] },
          { counterparty: "", transactions: [c] },
        ],
      }),
    ).toEqual([
      { uuid: a, counterparty: "Geç Tedarikçi" },
      { uuid: b, counterparty: "Geç Tedarikçi" },
      { uuid: c },
    ]);
  });
  it("reads a flat list and drops duplicates and non-uuids", () => {
    expect(
      warningTransactions({ transactions: [a, a, "x", 5, null, b] }),
    ).toEqual([{ uuid: a }, { uuid: b }]);
  });
  it("is empty for warnings without movements", () => {
    expect(warningTransactions(null)).toEqual([]);
    expect(warningTransactions({ total: "1" })).toEqual([]);
    expect(warningTransactions({ groups: "x", transactions: {} })).toEqual([]);
  });
});

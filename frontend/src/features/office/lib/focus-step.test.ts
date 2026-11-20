import { describe, expect, it } from "vitest";

import { focusNeighbor } from "./focus-step";

const href = (kind: string, id: string, extra = "") =>
  `/t/demo/engagements/e1/bank?${extra}kayit=${encodeURIComponent(`${kind}:${id}`)}`;

describe("focusNeighbor (#564)", () => {
  const rows = [
    href("bank_transaction", "a", "match=matched&"),
    href("bank_transaction", "a", "match=matched&"), // aynı satırın ikinci bağlantısı
    href("invoice", "x"),
    href("bank_transaction", "b", "match=matched&"),
    href("bank_transaction", "c", "match=matched&"),
  ];
  const at = (uuid: string) => ({ kind: "bank_transaction", uuid });

  it("steps to the next and previous row of the same kind", () => {
    expect(focusNeighbor(rows, at("a"), 1)).toBe(rows[3]);
    expect(focusNeighbor(rows, at("b"), 1)).toBe(rows[4]);
    expect(focusNeighbor(rows, at("b"), -1)).toBe(rows[0]);
  });
  it("stops at the ends", () => {
    expect(focusNeighbor(rows, at("a"), -1)).toBeNull();
    expect(focusNeighbor(rows, at("c"), 1)).toBeNull();
  });
  it("is null when the record is not listed on the page", () => {
    expect(focusNeighbor(rows, at("zzz"), 1)).toBeNull();
    expect(focusNeighbor([], at("a"), 1)).toBeNull();
  });
});

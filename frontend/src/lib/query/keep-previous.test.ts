import { describe, expect, it } from "vitest";

import { keepPreviousWithin } from "./keep-previous";

describe("keepPreviousWithin", () => {
  it("aynı mükellefte önceki sonucu tutar", () => {
    const keep = keepPreviousWithin("eng-a");
    expect(keep([1], { queryKey: ["tenant", "demo", "eng-a", "q=1"] })).toEqual(
      [1],
    );
  });

  it("mükellef değişince önceki mükellefin verisini göstermez (#572)", () => {
    const keep = keepPreviousWithin("eng-b");
    expect(
      keep([1], { queryKey: ["tenant", "demo", "eng-a"] }),
    ).toBeUndefined();
    expect(
      keepPreviousWithin(undefined)([1], { queryKey: ["x"] }),
    ).toBeUndefined();
  });
});

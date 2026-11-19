import { describe, expect, it } from "vitest";

import { trPossessive } from "./tr-suffix";

describe("trPossessive", () => {
  it.each([
    [0, "0'ı"],
    [1, "1'i"],
    [2, "2'si"],
    [3, "3'ü"],
    [6, "6'sı"],
    [9, "9'u"],
    [10, "10'u"],
    [20, "20'si"],
    [40, "40'ı"],
    [50, "50'si"],
    [90, "90'ı"],
    [95, "95'i"],
    [100, "100'ü"],
    [1000, "1000'i"],
  ])("%i", (n, want) => {
    expect(trPossessive(n)).toBe(want);
  });
});

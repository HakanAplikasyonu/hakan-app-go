import { describe, expect, it } from "vitest";

import { cleanValues, isWaiting } from "./use-einvoice-connections";

describe("cleanValues", () => {
  it("trims values and drops empty ones", () => {
    expect(
      cleanValues({
        username: " ofis ",
        password: "s3cret",
        sender_alias: "  ",
      }),
    ).toEqual({ username: "ofis", password: "s3cret" });
  });
});

describe("isWaiting", () => {
  it("is true only while the first sync runs", () => {
    expect(isWaiting({ state: "waiting" })).toBe(true);
    expect(isWaiting({ state: "active" })).toBe(false);
    expect(isWaiting({ state: "draft" })).toBe(false);
  });
});

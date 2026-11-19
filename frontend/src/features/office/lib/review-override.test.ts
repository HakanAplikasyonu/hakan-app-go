import { describe, expect, it } from "vitest";

import { engagementOverride, thresholdPercent } from "./review-override";

describe("engagementOverride", () => {
  it("finds the taxpayer's own catch-all row", () => {
    const row = engagementOverride([
      { level: "office", kind: "*", auto_accept: true, threshold: "0.95" },
      { level: "engagement", kind: "*", auto_accept: false, threshold: null },
    ]);
    expect(row?.auto_accept).toBe(false);
  });

  it("is null when only the office or a single kind is set", () => {
    expect(
      engagementOverride([
        { level: "office", kind: "*", auto_accept: true, threshold: "0.9" },
        {
          level: "engagement",
          kind: "account_code",
          auto_accept: true,
          threshold: "0.99",
        },
      ]),
    ).toBeNull();
  });
});

describe("thresholdPercent", () => {
  it("converts a ratio to a whole percent", () => {
    expect(thresholdPercent("0.95")).toBe(95);
    expect(thresholdPercent("0.9")).toBe(90);
  });

  it("falls back to 95 on a missing or broken value", () => {
    expect(thresholdPercent(null)).toBe(95);
    expect(thresholdPercent("x")).toBe(95);
  });
});

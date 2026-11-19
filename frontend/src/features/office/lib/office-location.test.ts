import { describe, expect, it } from "vitest";

import {
  engagementInPath,
  matchByPath,
  swapEngagement,
} from "./office-location";

describe("engagementInPath", () => {
  it("reads the open taxpayer from the address", () => {
    expect(engagementInPath("/t/demo/engagements/e-1/bank", "demo")).toBe(
      "e-1",
    );
    expect(engagementInPath("/t/demo/engagements/e-1", "demo")).toBe("e-1");
  });

  it("is null outside a taxpayer or another office", () => {
    expect(engagementInPath("/t/demo/warnings", "demo")).toBeNull();
    expect(engagementInPath("/t/other/engagements/e-1", "demo")).toBeNull();
    expect(engagementInPath("/platform", "demo")).toBeNull();
  });
});

describe("swapEngagement", () => {
  it("keeps the tab and changes the taxpayer", () => {
    expect(swapEngagement("/t/demo/engagements/e-1/bank", "demo", "e-2")).toBe(
      "/t/demo/engagements/e-2/bank",
    );
  });

  it("drops record paths below the tab", () => {
    expect(
      swapEngagement("/t/demo/engagements/e-1/invoices/i-9", "demo", "e-2"),
    ).toBe("/t/demo/engagements/e-2/invoices");
  });

  it("opens the overview when no tab is open", () => {
    expect(swapEngagement("/t/demo/engagements/e-1", "demo", "e-2")).toBe(
      "/t/demo/engagements/e-2",
    );
    expect(swapEngagement("/t/demo/warnings", "demo", "e-2")).toBe(
      "/t/demo/engagements/e-2",
    );
  });
});

describe("matchByPath", () => {
  const items = [
    { href: "/t/demo", exact: true },
    { href: "/t/demo/settings" },
    { href: "/t/demo/settings/roles" },
    { href: "/t/demo/review?tab=open" },
  ];

  it("prefers the longest matching link", () => {
    expect(matchByPath("/t/demo/settings/roles/3", items)?.href).toBe(
      "/t/demo/settings/roles",
    );
    expect(matchByPath("/t/demo/settings", items)?.href).toBe(
      "/t/demo/settings",
    );
  });

  it("matches an exact link only on the same address", () => {
    expect(matchByPath("/t/demo", items)?.href).toBe("/t/demo");
    expect(matchByPath("/t/demo/unknown", items)).toBeUndefined();
  });

  it("ignores the query of a link", () => {
    expect(matchByPath("/t/demo/review", items)?.href).toBe(
      "/t/demo/review?tab=open",
    );
  });
});

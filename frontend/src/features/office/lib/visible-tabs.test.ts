import { describe, expect, it } from "vitest";

import { visibleTabs } from "./visible-tabs";

describe("visibleTabs", () => {
  const tabs = [
    { href: "/overview" },
    { href: "/bank", permission: "tenant.bank.read" },
    { href: "/review", permission: "tenant.review.read" },
  ];

  it("keeps open tabs and the ones the role may read", () => {
    const can = (p: string) => p === "tenant.bank.read";
    expect(visibleTabs(tabs, can)).toEqual([
      { href: "/overview" },
      { href: "/bank" },
    ]);
  });

  it("hides every gated tab from a role without permissions", () => {
    expect(visibleTabs(tabs, () => false)).toEqual([{ href: "/overview" }]);
  });
});

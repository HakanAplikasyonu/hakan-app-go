import { describe, expect, it } from "vitest";

import {
  findGuide,
  guidesFor,
  helpRoles,
  primaryGuide,
  PUBLIC_GUIDES,
} from "./guides";

describe("helpRoles", () => {
  it("is empty for a visitor", () => {
    expect(helpRoles(null)).toEqual([]);
  });

  it("collects office roles, taxpayer membership and platform", () => {
    expect(
      helpRoles({
        organizations: [
          { role: { key: "owner" } },
          { role: { key: "intern" } },
        ],
        taxpayers: [{}],
      }).sort(),
    ).toEqual(["intern", "owner", "taxpayer"]);
    expect(helpRoles({ isSuperAdmin: true })).toEqual(["platform"]);
  });
});

describe("guidesFor", () => {
  it("never lists internal guides", () => {
    expect(PUBLIC_GUIDES.some((g) => g.internal)).toBe(false);
    expect(PUBLIC_GUIDES.map((g) => g.slug)).not.toContain("demo-senaryosu");
    // Bağlantısı olan yine açabilir.
    expect(findGuide("demo-senaryosu")?.internal).toBe(true);
  });

  it("puts the owner's guide first and keeps the rest reachable", () => {
    const { mine, others } = guidesFor(["owner"]);
    expect(mine[0]?.slug).toBe("ofis-yoneticisi");
    expect(mine.map((g) => g.slug)).not.toContain("mukellef");
    expect(others.map((g) => g.slug)).toContain("mukellef");
    expect(mine.length + others.length).toBe(PUBLIC_GUIDES.length);
  });

  it("gives each role its own primary guide", () => {
    expect(primaryGuide(["admin"])?.slug).toBe("ofis-yoneticisi");
    expect(primaryGuide(["accountant"])?.slug).toBe("ofis-calisani");
    expect(primaryGuide(["intern"])?.slug).toBe("ofis-calisani");
    expect(primaryGuide(["taxpayer"])?.slug).toBe("mukellef");
  });

  it("shows the general list to a visitor and to a platform admin", () => {
    expect(guidesFor([]).mine).toEqual([]);
    expect(guidesFor([]).others.length).toBe(PUBLIC_GUIDES.length);
    expect(primaryGuide(["platform"])).toBeUndefined();
  });
});

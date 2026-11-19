import { describe, expect, it } from "vitest";

import {
  HELP_CONTEXT_SECTIONS,
  helpHref,
  helpTarget,
  pageKey,
} from "./context";
import { findGuide } from "./guides";
import { guideOutline } from "./outline";

describe("pageKey", () => {
  it("reads the office page and the taxpayer tab", () => {
    expect(pageKey("/t/demo/warnings")).toBe("warnings");
    expect(pageKey("/t/demo/engagements/e-1/bank")).toBe("bank");
    expect(pageKey("/t/demo/engagements/e-1")).toBeNull();
    expect(pageKey("/t/demo")).toBeNull();
    expect(pageKey("/m/abc/receipts")).toBeNull();
  });
});

describe("helpTarget", () => {
  it("opens the role's guide at the section of the current page", () => {
    expect(helpTarget(["accountant"], "/t/demo/warnings")).toEqual({
      slug: "ofis-calisani",
      section: "uyarılar",
    });
    expect(helpTarget(["owner"], "/t/demo/engagements/e-1/journals")).toEqual({
      slug: "ofis-yoneticisi",
      section: "7-yevmiye-ve-luca--zirve-çıktısı",
    });
  });

  it("opens the guide's start on a page without a section", () => {
    expect(helpTarget(["intern"], "/t/demo/agents")).toEqual({
      slug: "ofis-calisani",
      section: undefined,
    });
    expect(helpTarget(["taxpayer"])).toEqual({
      slug: "mukellef",
      section: undefined,
    });
  });

  it("falls back to the help center without a role guide", () => {
    expect(helpTarget([], "/t/demo/warnings")).toBeNull();
    expect(helpHref(null)).toBe("/yardim");
  });

  it("builds an encoded address", () => {
    expect(helpHref({ slug: "ofis-calisani", section: "uyarılar" })).toBe(
      `/yardim/ofis-calisani#${encodeURIComponent("uyarılar")}`,
    );
  });
});

describe("context sections", () => {
  it("point at sections that exist in the guides", () => {
    for (const [slug, map] of Object.entries(HELP_CONTEXT_SECTIONS)) {
      const guide = findGuide(slug);
      expect(guide, slug).toBeDefined();
      const ids = guideOutline(guide!.body).sections.map((s) => s.id);
      for (const [page, section] of Object.entries(map))
        expect(ids, `${slug} / ${page}`).toContain(section);
    }
  });
});

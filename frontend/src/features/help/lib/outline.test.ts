import { describe, expect, it } from "vitest";

import { findGuide, PUBLIC_GUIDES } from "./guides";
import { guideOutline, headingId, plainText, searchGuides } from "./outline";

describe("headingId", () => {
  it("follows the GitHub rule and keeps Turkish letters", () => {
    expect(headingId("UBL yükleme")).toBe("ubl-yükleme");
    expect(headingId("7. Yevmiye ve Luca / Zirve çıktısı")).toBe(
      "7-yevmiye-ve-luca--zirve-çıktısı",
    );
    expect(headingId("İnceleme")).toBe("inceleme");
  });

  it("numbers repeated headings", () => {
    const used = new Map<string, number>();
    expect(headingId("Kurulum", used)).toBe("kurulum");
    expect(headingId("Kurulum", used)).toBe("kurulum-1");
  });
});

describe("guideOutline", () => {
  const body = [
    "# Başlık",
    "",
    "> Kısa özet.",
    "",
    "Giriş metni.",
    "",
    "## Birinci",
    "",
    "Metin.",
    "",
    "```",
    "## kod içinde başlık değil",
    "```",
    "",
    "## İkinci",
    "",
    "Son.",
  ].join("\n");

  it("splits title, summary, intro and sections", () => {
    const o = guideOutline(body);
    expect(o.title).toBe("Başlık");
    expect(o.summary).toBe("Kısa özet.");
    expect(o.intro).toBe("Giriş metni.");
    expect(o.sections.map((s) => s.id)).toEqual(["birinci", "ikinci"]);
    expect(o.sections[0]?.source).toContain("## kod içinde başlık değil");
  });

  it("gives every real guide unique section ids", () => {
    for (const g of PUBLIC_GUIDES) {
      const ids = guideOutline(g.body).sections.map((s) => s.id);
      expect(ids.length, g.slug).toBeGreaterThan(0);
      expect(new Set(ids).size, g.slug).toBe(ids.length);
    }
  });

  it("resolves the cross-guide anchors used in the guides", () => {
    for (const g of PUBLIC_GUIDES) {
      for (const m of g.body.matchAll(
        /\]\((?:\.\/)?([a-z0-9-]+)\.md#([^)]+)\)/g,
      )) {
        const target = findGuide(m[1]!);
        expect(target, `${g.slug} → ${m[1]}`).toBeDefined();
        const ids = guideOutline(target!.body).sections.map((s) => s.id);
        expect(ids, `${g.slug} → ${m[1]}#${m[2]}`).toContain(
          decodeURIComponent(m[2]!),
        );
      }
    }
  });
});

describe("plainText", () => {
  it("drops markdown marks, images and code", () => {
    expect(
      plainText("- **Kalın** [bağlantı](x.md) ![resim](a.jpg)\n```\nkod\n```"),
    ).toBe("Kalın bağlantı");
  });
});

describe("searchGuides", () => {
  it("returns nothing for a blank or one-letter query", () => {
    expect(searchGuides(" ", PUBLIC_GUIDES)).toEqual([]);
    expect(searchGuides("a", PUBLIC_GUIDES)).toEqual([]);
  });

  it("finds a section by a word in its title, case-insensitively in Turkish", () => {
    const hits = searchGuides("İNCELEME", PUBLIC_GUIDES);
    expect(hits.length).toBeGreaterThan(0);
    expect(hits[0]?.inTitle).toBe(true);
    expect(hits.some((h) => h.section?.id === "inceleme")).toBe(true);
  });

  it("needs every word in the same section and returns a snippet", () => {
    const hits = searchGuides("dosya türü izinli", PUBLIC_GUIDES);
    expect(hits.length).toBeGreaterThan(0);
    expect(hits[0]?.snippet.length).toBeGreaterThan(10);
    expect(searchGuides("zzqq yokboyle", PUBLIC_GUIDES)).toEqual([]);
  });
});

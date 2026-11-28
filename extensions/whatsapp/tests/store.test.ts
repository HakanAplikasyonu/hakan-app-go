import { readFileSync } from "node:fs";
import { join } from "node:path";
import { describe, expect, it } from "vitest";

// @ts-expect-error: düz JS betiği (scripts/store-manifest.mjs), tip bildirimi yok.
import { storeManifest, storeOrigin, storeProblems } from "../scripts/store-manifest.mjs";

const source = JSON.parse(readFileSync(join(__dirname, "../static/manifest.json"), "utf8"));
const built = { ...source, version: "0.2.0" };

describe("mağaza manifesti", () => {
  it("isteğe bağlı konak iznini gömülü sunucuya daraltır, kalan izinleri korur", () => {
    const m = storeManifest(built, "https://api.ofis.example/v1");
    expect(m.optional_host_permissions).toEqual(["https://api.ofis.example/*"]);
    expect(m.permissions).toEqual(["storage"]);
    expect(m.host_permissions).toEqual(["https://web.whatsapp.com/*"]);
    expect(storeProblems(m)).toEqual([]);
    // Girdi değişmez (panel paketi geniş izinle kalır).
    expect(built.optional_host_permissions).toContain("https://*/*");
  });

  it("kaynak manifest mağazada geniş izin sayılır", () => {
    expect(storeProblems(built).join(" ")).toContain("https://*/*");
  });

  it("https olmayan ya da yerel adresi reddeder", () => {
    expect(() => storeOrigin("http://api.ofis.example")).toThrow(/https/);
    expect(() => storeOrigin("https://localhost:8080")).toThrow(/yerel/);
    expect(() => storeOrigin("")).toThrow();
  });

  it("gereksiz izin, uzun açıklama ve eksik sürüm yakalanır", () => {
    const m = storeManifest(built, "https://api.ofis.example");
    const bad = { ...m, permissions: ["storage", "tabs"], description: "x".repeat(133), version: "0.0.0" };
    const p = storeProblems(bad).join(" | ");
    expect(p).toContain("gereksiz izin: tabs");
    expect(p).toContain("132");
    expect(p).toContain("version");
  });

  it("kaynak açıklama 132 karakteri aşmaz", () => {
    expect(source.description.length).toBeLessThanOrEqual(132);
  });
});

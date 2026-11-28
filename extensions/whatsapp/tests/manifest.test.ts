import { readFileSync } from "node:fs";
import { createHash } from "node:crypto";
import { join } from "node:path";
import { describe, expect, it } from "vitest";

// @ts-expect-error: düz JS betiği (scripts/manifest.mjs), tip bildirimi yok.
import { finalizeManifest, originPattern, panelFromApi } from "../scripts/manifest.mjs";

const root = join(__dirname, "..");
const base = JSON.parse(readFileSync(join(root, "static/manifest.json"), "utf8"));
const key = readFileSync(join(root, "scripts/extension-key.txt"), "utf8");

/** Chrome'un genel anahtardan uzantı kimliği türetmesi. */
function extensionId(pub: string): string {
  const hex = createHash("sha256").update(Buffer.from(pub.trim(), "base64")).digest("hex").slice(0, 32);
  return [...hex].map((c) => String.fromCharCode(97 + parseInt(c, 16))).join("");
}

describe("finalizeManifest", () => {
  it("fixes the unpacked id with the committed public key", () => {
    const m = finalizeManifest(base, { version: "0.3.0", key });
    expect(m.version).toBe("0.3.0");
    // Panel varsayılanı (frontend whatsapp-setup.ts DEFAULT_EXTENSION_IDS) ile aynı.
    expect(extensionId(m.key)).toBe("fbcngkcogndfncloaedcfpkofebbfhco");
    expect(finalizeManifest(base, { version: "0.3.0", key, noKey: true }).key).toBeUndefined();
  });
  it("lets only the panel and local hosts ask whether it is installed", () => {
    const m = finalizeManifest(base, { version: "1", key, apiUrl: "https://api.ofis.example" });
    expect(m.externally_connectable.matches).toEqual(["http://localhost/*", "http://127.0.0.1/*", "https://app.ofis.example/*"]);
    expect(m.host_permissions).toContain("https://api.ofis.example/*");
    const explicit = finalizeManifest(base, { version: "1", key, apiUrl: "https://api.ofis.example", panelUrls: "https://panel.ofis.example" });
    expect(explicit.externally_connectable.matches).toContain("https://panel.ofis.example/*");
    expect(explicit.externally_connectable.matches).not.toContain("https://app.ofis.example/*");
  });
  it("treats a same-origin /api address as the panel", () => {
    const m = finalizeManifest(base, { version: "1", key, apiUrl: "https://app.ofis.example/api" });
    expect(m.externally_connectable.matches).toContain("https://app.ofis.example/*");
  });
  it("builds patterns", () => {
    expect(originPattern("http://127.0.0.1:8080/v1")).toBe("http://127.0.0.1/*");
    expect(originPattern("ftp://x")).toBeNull();
    expect(panelFromApi("https://api.ofis.example/v1")).toBe("https://app.ofis.example/");
  });
});

import { readFileSync } from "node:fs";
import { join } from "node:path";
import { describe, expect, it, vi } from "vitest";

const script = readFileSync(join(__dirname, "..", "scripts", "tani.js"), "utf8");
const fixture = readFileSync(join(__dirname, "fixtures", "chat-2026-11.html"), "utf8");

function run(): Record<string, any> {
  const log = vi.spyOn(console, "log").mockImplementation(() => {});
  const out = new Function(`return (\n${script.trim().replace(/;$/, "")}\n);`)();
  expect(log).toHaveBeenCalledTimes(1);
  log.mockRestore();
  return out;
}

describe("scripts/tani.js", () => {
  it("summarises the DOM without message text, masking digits", () => {
    document.body.innerHTML = fixture;
    const spyText = vi.spyOn(Node.prototype, "textContent", "get");
    const out = run();
    expect(spyText).not.toHaveBeenCalled();
    spyText.mockRestore();
    const json = JSON.stringify(out);
    expect(json).not.toMatch(/GİZLİ|Deniz|aidat|10:1|blob:https|6666/);
    expect(json).not.toMatch(/pre-plain-text/);
    expect(out.main).toBe(1);
    expect(out.dataIdOrnekleri).toContain("X");
    expect(out.secici['[data-testid="document-thumb"]']).toBe(2);
    expect(out.dataIcon).toContain("document-pdf-icon");
    expect(out.sonSatir.attrs["data-testid"]).toBe("conv-msg-X");
  });

  it("reports only the suffix of the active chat id", () => {
    document.body.innerHTML = fixture;
    const w = window as unknown as Record<string, unknown>;
    w.require = (m: string) => {
      if (m !== "WAWebChatCollection") throw new Error("unknown");
      return { ChatCollection: { getActive: () => ({ id: { _serialized: "98765432101234@lid" }, contact: { phoneNumber: { _serialized: "905551112233@c.us" } } }) } };
    };
    const out = run();
    delete w.require;
    expect(out.sohbetErisimcisi["require(WAWebChatCollection).ChatCollection.getActive"]).toEqual({ id: "@lid", phoneNumber: "@c.us", hasTitle: false });
    expect(out.sohbetErisimcisi["require(WAWebCollections).Chat.getActive"]).toMatch(/^yok/);
    expect(JSON.stringify(out)).not.toMatch(/90555|98765/);
  });
});

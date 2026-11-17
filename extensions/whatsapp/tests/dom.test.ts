import { readFileSync } from "node:fs";
import { join } from "node:path";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { currentChat, documentDownloadControl, findMedia, findViewer, mediaOf, parseChatJid } from "../src/dom";
import { BAR_CLASS, attachBar } from "../src/ui";

const fixture = (name: string) => readFileSync(join(__dirname, "fixtures", name), "utf8");

function load(name: string) {
  document.body.innerHTML = fixture(name);
}

describe("parseChatJid", () => {
  it.each([
    ["false_905551112233@c.us_3EB0A1B2C3", "905551112233@c.us"],
    ["true_905551112233@c.us_3EB0A1B2C3", "905551112233@c.us"],
    ["false_120363041234567890@g.us_ABCD_905551112233@c.us", "120363041234567890@g.us"],
    ["false_98765432101234@lid_3EB0", "98765432101234@lid"],
  ])("%s → %s", (id, jid) => expect(parseChatJid(id)).toBe(jid));

  it.each(["", "3EB0A1B2C3", "false_status@broadcast_1", "x_905551112233@c.us_1"])("rejects %s", (id) => {
    expect(parseChatJid(id)).toBeNull();
  });
});

describe("chat fixture", () => {
  beforeEach(() => load("chat.html"));

  it("reads the chat id and title", () => {
    expect(currentChat(document)).toEqual({ jid: "905321234567@c.us", title: "Okan Atlas (Atlas Gıda)" });
  });

  it("finds only savable media", () => {
    const items = findMedia(document);
    const summary = items.map((m) => [m.row.getAttribute("data-id")?.split("_")[2], m.kind, m.src !== null, m.receiptable, m.fileName]);
    expect(summary).toEqual([
      ["3EB0IMAGE001", "image", true, true, ""],
      ["3EB0PREVIEW1", "image", false, true, ""], // not downloaded yet
      ["3EB0DOCPDF01", "document", false, false, "Ekim kira faturası.pdf"], // PDF: document only
      ["3EB0DOCJPG01", "document", false, true, "fis-tarama.JPG"],
    ]);
    // Text, sticker, emoji and .docx messages get no buttons.
  });

  it("finds the document's download control", () => {
    const pdf = findMedia(document).find((m) => m.fileName.endsWith(".pdf"))!;
    expect(documentDownloadControl(pdf.row)?.getAttribute("aria-label")).toBe("İndir");
  });

  it("adds the bar once, with the right buttons", () => {
    const onSave = vi.fn();
    for (const m of findMedia(document)) attachBar(m, { onSave });
    for (const m of findMedia(document)) expect(attachBar(m, { onSave })).toBeNull();
    const bars = document.querySelectorAll(`.${BAR_CLASS}`);
    expect(bars).toHaveLength(4);
    const labels = (i: number) => Array.from(bars[i]!.shadowRoot!.querySelectorAll("button")).map((b) => b.textContent);
    expect(labels(0)).toEqual(["Gider olarak kaydet", "Gelir olarak kaydet", "Belge olarak kaydet"]);
    expect(labels(2)).toEqual(["Belge olarak kaydet"]);
    (bars[0]!.shadowRoot!.querySelector('button[data-target="income"]') as HTMLButtonElement).click();
    expect(onSave).toHaveBeenCalledWith("income", expect.objectContaining({ kind: "image" }));
    // Our own nodes do not turn into media.
    expect(findMedia(document)).toHaveLength(4);
  });

  it("never reads message text", () => {
    // dom.ts reads attributes only: watch textContent (jsdom has no innerText;
    // dom.ts does not use it either, see the source check below).
    const spyText = vi.spyOn(Node.prototype, "textContent", "get");
    const items = findMedia(document);
    const chat = currentChat(document);
    for (const m of items) mediaOf(m.row);
    expect(spyText).not.toHaveBeenCalled();
    expect(JSON.stringify({ chat, items: items.map((m) => ({ ...m, row: undefined })) })).not.toMatch(/GİZLİ/);
    spyText.mockRestore();
  });

  it("has no text readers in the page modules", () => {
    for (const f of ["dom.ts", "selectors.ts"]) {
      const src = readFileSync(join(__dirname, "..", "src", f), "utf8").replace(/\/\/.*$/gm, "");
      expect(src, f).not.toMatch(/\.(textContent|innerText|innerHTML|outerHTML)\b/);
    }
  });
});

describe("group fixture", () => {
  it("uses the group id, not the participant", () => {
    load("group.html");
    expect(currentChat(document)?.jid).toBe("120363041234567890@g.us");
    expect(findMedia(document)).toHaveLength(1);
  });
});

describe("viewer fixture", () => {
  it("finds the full-size image and the toolbar", () => {
    load("viewer.html");
    const v = findViewer(document)!;
    expect(v.src).toBe("blob:https://web.whatsapp.com/55555555-5555-5555-5555-555555555555");
    expect(v.toolbar.getAttribute("data-testid")).toBe("media-viewer-toolbar");
  });

  it("is absent without a viewer", () => {
    load("chat.html");
    expect(findViewer(document)).toBeNull();
  });
});

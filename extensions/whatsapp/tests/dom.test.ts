import { readFileSync } from "node:fs";
import { join } from "node:path";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { chatTitle, currentChat, documentDownloadControl, findMedia, findViewer, legacyChat, mediaOf, parseChatJid } from "../src/dom";
import { BAR_CLASS, attachBar } from "../src/ui";

const fixture = (name: string) => readFileSync(join(__dirname, "fixtures", name), "utf8");

function load(name: string) {
  document.body.innerHTML = fixture(name);
}

// Sayfanın dünyası yanıt vermiyor (eski WhatsApp ya da MAIN betiği yok).
const noPage = async () => null;

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

  it("reads the chat id and title", async () => {
    expect(await currentChat(document, noPage)).toEqual({ jid: "905321234567@c.us", title: "Okan Atlas (Atlas Gıda)" });
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
    expect(labels(0)).toEqual(["Gider olarak kaydet", "Z raporu olarak kaydet", "Belge olarak kaydet", "Yabancı fatura olarak kaydet"]);
    expect(labels(2)).toEqual(["Belge olarak kaydet", "Yabancı fatura olarak kaydet"]);
    (bars[0]!.shadowRoot!.querySelector('button[data-target="income"]') as HTMLButtonElement).click();
    expect(onSave).toHaveBeenCalledWith("income", expect.objectContaining({ kind: "image" }));
    // Our own nodes do not turn into media.
    expect(findMedia(document)).toHaveLength(4);
  });

  it("never reads message text", async () => {
    // dom.ts reads attributes only: watch textContent (jsdom has no innerText;
    // dom.ts does not use it either, see the source check below).
    const spyText = vi.spyOn(Node.prototype, "textContent", "get");
    const items = findMedia(document);
    const chat = await currentChat(document, noPage);
    for (const m of items) mediaOf(m.row);
    expect(spyText).not.toHaveBeenCalled();
    expect(JSON.stringify({ chat, items: items.map((m) => ({ ...m, row: undefined })) })).not.toMatch(/GİZLİ/);
    spyText.mockRestore();
  });

  it("has no text readers in the page modules", () => {
    for (const f of ["dom.ts", "selectors.ts", "chat.ts", "main-world.ts", "intercept.ts"]) {
      const src = readFileSync(join(__dirname, "..", "src", f), "utf8").replace(/\/\/.*$/gm, "");
      expect(src, f).not.toMatch(/\.(textContent|innerText|innerHTML|outerHTML)\b/);
      expect(src, f).not.toMatch(/pre-plain-text|selectable-text|prePlainText/);
    }
  });
});

describe("group fixture", () => {
  it("uses the group id, not the participant", async () => {
    load("group.html");
    expect((await currentChat(document, noPage))?.jid).toBe("120363041234567890@g.us");
    expect(findMedia(document)).toHaveLength(1);
  });
});

describe("2026-11 chat fixture (data-id = message id only)", () => {
  beforeEach(() => load("chat-2026-11.html"));

  it("finds media without a chat id in data-id", () => {
    const summary = findMedia(document).map((m) => [m.row.getAttribute("data-id"), m.kind, m.src !== null, m.receiptable, m.fileName]);
    expect(summary).toEqual([
      ["AC1F00D2E3B4C5D6E7F8", "image", true, true, ""],
      ["B0B0B0B0C1C1C1C1D2D2", "image", false, true, ""],
      ["D0C0FFEE000011112222", "document", false, false, "Kasım aidat makbuzu.pdf"],
    ]);
    // Text, grouped stickers and .xlsx get no buttons.
  });

  it("rows are the outer conv-msg elements, not nested ones", () => {
    for (const m of findMedia(document)) expect(m.row.getAttribute("data-testid")).toMatch(/^conv-msg-/);
  });

  it("clicks the media-state-download control for documents", () => {
    const pdf = findMedia(document).find((m) => m.kind === "document")!;
    expect(documentDownloadControl(pdf.row)?.getAttribute("data-testid")).toBe("media-state-download");
  });

  it("falls back to the document bubble without a download control", () => {
    const pdf = findMedia(document).find((m) => m.kind === "document")!;
    pdf.row.querySelector('[data-testid="media-state-download"]')!.remove();
    expect(documentDownloadControl(pdf.row)?.getAttribute("data-testid")).toBe("document-thumb");
  });

  it("legacy data-id parsing finds no chat; the page world provides it", async () => {
    expect(legacyChat(document)).toBeNull();
    expect(await currentChat(document, noPage)).toBeNull();
    expect(await currentChat(document, async () => ({ jid: "905551112233@c.us", title: "" }))).toEqual({
      jid: "905551112233@c.us",
      title: "Deniz Yel (Yel Kırtasiye)",
    });
    expect(await currentChat(document, async () => ({ jid: "120363000000000000@g.us", title: "Yel Kırtasiye Muhasebe" }))).toEqual({
      jid: "120363000000000000@g.us",
      title: "Yel Kırtasiye Muhasebe",
    });
  });

  it("reads the title from the header attributes only", () => {
    expect(chatTitle(document)).toBe("Deniz Yel (Yel Kırtasiye)");
  });

  it("never reads message text", async () => {
    const spyText = vi.spyOn(Node.prototype, "textContent", "get");
    const spyAttr = vi.spyOn(Element.prototype, "getAttribute");
    const items = findMedia(document);
    const chat = await currentChat(document, noPage);
    for (const m of items) {
      mediaOf(m.row);
      documentDownloadControl(m.row);
    }
    chatTitle(document);
    expect(spyText).not.toHaveBeenCalled();
    expect(spyAttr.mock.calls.map((c) => c[0])).not.toContain("data-pre-plain-text");
    expect(JSON.stringify({ chat, items: items.map((m) => ({ ...m, row: undefined })) })).not.toMatch(/GİZLİ|10:1|03\.11/);
    spyText.mockRestore();
    spyAttr.mockRestore();
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

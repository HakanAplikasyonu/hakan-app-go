import { describe, expect, it, vi } from "vitest";

import { fileNameFor, resultMessage, save, toBase64, type FlowDeps } from "../src/flow";
import type { Capture, ChatLookup, OpenTask, Reply, Request, Taxpayer } from "../src/messages";

const atlas: Taxpayer = { engagement_uuid: "eng-1", taxpayer_uuid: "tp-1", title: "Atlas Gıda", tax_id: "9000000017" };
const chat = { jid: "905321234567@c.us", title: "Okan Atlas" };
const pdf = { blob: new Blob(["%PDF-1.4"], { type: "application/pdf" }), name: "kira.pdf" };

function capture(over: Partial<Capture> = {}): Capture {
  return {
    uuid: "c1", target: "document", file_name: "kira.pdf", taxpayer: atlas, receipt_uuid: null, receipt_status: null,
    task_uuid: null, task_title: null, has_file: true, replayed: false, ...over,
  };
}

function deps(lookup: ChatLookup, tasks: OpenTask[] = [], picked: Taxpayer | null = atlas, task?: OpenTask | null) {
  const sent: Request[] = [];
  const d: FlowDeps = {
    send: async <T>(req: Request): Promise<Reply<T>> => {
      sent.push(req);
      switch (req.type) {
        case "lookup":
          return { ok: true, data: lookup as T };
        case "link":
          return { ok: true, data: lookup as T };
        case "tasks":
          return { ok: true, data: tasks as T };
        case "capture":
          return { ok: true, data: capture({ target: req.target, task_uuid: req.taskUuid ?? null, task_title: req.taskUuid ? "Kira sözleşmesi" : null }) as T };
        default:
          return { ok: false, error: "unexpected" };
      }
    },
    pickTaxpayer: vi.fn(async () => (picked ? { taxpayer: picked, remember: true } : null)),
    pickTask: vi.fn(async () => task),
  };
  return { d, sent };
}

describe("save", () => {
  it("uses the remembered taxpayer without asking", async () => {
    const { d, sent } = deps({ chat_key: "905321234567", link: { uuid: "l", chat_key: "905321234567", label: "", taxpayer: atlas }, suggestions: [] });
    const res = await save(d, "expense", chat, { blob: new Blob(["x"], { type: "image/jpeg" }), name: "" });
    expect(res.status).toBe("saved");
    expect(d.pickTaxpayer).not.toHaveBeenCalled();
    expect(sent.map((r) => r.type)).toEqual(["lookup", "capture"]);
    const c = sent[1] as Extract<Request, { type: "capture" }>;
    expect(c).toMatchObject({ engagementUuid: "eng-1", target: "expense", chatKey: "905321234567@c.us", mimeType: "image/jpeg" });
    expect(c.fileName).toMatch(/^whatsapp-905321234567-\d{4}-\d{2}-\d{2}\.jpg$/);
  });

  it("asks the first time and remembers the choice", async () => {
    const { d, sent } = deps({ chat_key: "905321234567", link: null, suggestions: [atlas] });
    const res = await save(d, "document", chat, pdf);
    expect(d.pickTaxpayer).toHaveBeenCalledWith("Okan Atlas", [atlas]);
    expect(sent.map((r) => r.type)).toEqual(["lookup", "link", "tasks", "capture"]);
    expect(sent[1]).toEqual({ type: "link", engagementUuid: "eng-1", chatKey: "905321234567@c.us", label: "Okan Atlas" });
    expect(res).toMatchObject({ status: "saved", message: "Atlas Gıda: belge kaydedildi." });
  });

  it("sends a document into the chosen open task", async () => {
    const task: OpenTask = { uuid: "task-1", title: "Kira sözleşmesi", period: "2026-10", status: "open" };
    const { d, sent } = deps({ chat_key: "x", link: { uuid: "l", chat_key: "x", label: "", taxpayer: atlas }, suggestions: [] }, [task], atlas, task);
    const res = await save(d, "document", chat, pdf);
    expect((sent.at(-1) as Extract<Request, { type: "capture" }>).taskUuid).toBe("task-1");
    expect(res).toMatchObject({ status: "saved", message: "Atlas Gıda: belge “Kira sözleşmesi” talebine eklendi." });
  });

  it("stops when the user cancels", async () => {
    const { d, sent } = deps({ chat_key: "x", link: null, suggestions: [] }, [], null);
    expect(await save(d, "income", chat, pdf)).toEqual({ status: "cancelled" });
    expect(sent.map((r) => r.type)).toEqual(["lookup"]);
  });

  it("reports the server's message", async () => {
    const d: FlowDeps = {
      send: async <T>(): Promise<Reply<T>> => ({ ok: false, error: "Bu işlem için yetkiniz yok." }),
      pickTaxpayer: vi.fn(),
      pickTask: vi.fn(),
    };
    expect(await save(d, "expense", chat, pdf)).toEqual({ status: "error", message: "Bu işlem için yetkiniz yok." });
  });
});

describe("helpers", () => {
  it("base64-encodes large files", async () => {
    const bytes = new Uint8Array(100_000).map((_, i) => i % 251);
    const b64 = await toBase64(new Blob([bytes]));
    expect(Buffer.from(b64, "base64").equals(Buffer.from(bytes))).toBe(true);
  });

  it("keeps the document's own name", () => {
    expect(fileNameFor(pdf, chat)).toBe("kira.pdf");
  });

  it("says when a file was already saved", () => {
    expect(resultMessage(capture({ replayed: true }))).toBe("Atlas Gıda: bu dosya daha önce kaydedilmiş.");
    expect(resultMessage(capture({ target: "income", receipt_status: "draft" }))).toMatch(/Z raporu/);
  });

  it("says a receipt read in the background is saved and being read (#780)", () => {
    expect(resultMessage(capture({ target: "expense", receipt_status: "processing" }))).toBe("Atlas Gıda: fiş kaydedildi, okunuyor.");
    expect(resultMessage(capture({ target: "income", receipt_status: "processing" }))).toBe("Atlas Gıda: Z raporu kaydedildi, okunuyor.");
  });
});

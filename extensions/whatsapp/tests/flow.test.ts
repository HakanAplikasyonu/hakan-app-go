import { describe, expect, it, vi } from "vitest";

import { fileNameFor, resultMessage, save, toBase64, type FlowDeps } from "../src/flow";
import type { CaptureStatus } from "../src/badge";
import type { Capture, ChatLookup, OpenTask, Reply, Request, Taxpayer } from "../src/messages";

const atlas: Taxpayer = { engagement_uuid: "eng-1", taxpayer_uuid: "tp-1", title: "Atlas Gıda", tax_id: "9000000017" };
const beta: Taxpayer = { engagement_uuid: "eng-2", taxpayer_uuid: "tp-2", title: "Beta Yapı", tax_id: "9000000025" };
const SHA = "a".repeat(64);
const chat = { jid: "905321234567@c.us", title: "Okan Atlas" };
const pdf = { blob: new Blob(["%PDF-1.4"], { type: "application/pdf" }), name: "kira.pdf" };

function capture(over: Partial<Capture> = {}): Capture {
  return {
    uuid: "c1", target: "document", file_name: "kira.pdf", taxpayer: atlas, receipt_uuid: null, receipt_status: null,
    task_uuid: null, task_title: null, has_file: true, replayed: false, ...over,
  };
}

interface Opts {
  savePhone?: boolean;
  phoneError?: string;
  statuses?: CaptureStatus[];
}

function deps(lookup: ChatLookup, tasks: OpenTask[] = [], picked: Taxpayer | null = atlas, task?: OpenTask | null, opts: Opts = {}) {
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
        case "savePhone":
          return opts.phoneError ? { ok: false, error: opts.phoneError } : { ok: true, data: true as T };
        case "statuses":
          return { ok: true, data: (opts.statuses ?? []) as T };
        case "capture":
          return { ok: true, data: capture({ target: req.target, task_uuid: req.taskUuid ?? null, task_title: req.taskUuid ? "Kira sözleşmesi" : null }) as T };
        default:
          return { ok: false, error: "unexpected" };
      }
    },
    pickTaxpayer: vi.fn(async () => (picked ? { taxpayer: picked, remember: true, savePhone: opts.savePhone ?? false } : null)),
    pickTask: vi.fn(async () => task),
    ...(opts.statuses ? { hash: async () => SHA } : {}),
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

  it("picks the taxpayer by phone when exactly one matches", async () => {
    const { d, sent } = deps({ chat_key: "905321234567", link: null, suggestions: [atlas] });
    const res = await save(d, "expense", chat, { blob: new Blob(["x"], { type: "image/jpeg" }), name: "" });
    expect(d.pickTaxpayer).not.toHaveBeenCalled();
    expect(sent.map((r) => r.type)).toEqual(["lookup", "capture"]);
    expect(res).toMatchObject({ status: "saved", autoMatched: true });
    expect((res as { message: string }).message).toMatch(/^Mükellef telefondan bulundu: Atlas Gıda\. /);
  });

  it("asks the first time and remembers the choice", async () => {
    const { d, sent } = deps({ chat_key: "905321234567", link: null, suggestions: [atlas, beta] });
    const res = await save(d, "document", chat, pdf);
    expect(d.pickTaxpayer).toHaveBeenCalledWith("Okan Atlas", [atlas, beta], "+905321234567");
    expect(sent.map((r) => r.type)).toEqual(["lookup", "link", "tasks", "capture"]);
    expect(sent[1]).toEqual({ type: "link", engagementUuid: "eng-1", chatKey: "905321234567@c.us", label: "Okan Atlas" });
    expect(res).toMatchObject({ status: "saved", message: "Atlas Gıda: belge kaydedildi." });
  });

  it("saves the chat number as the taxpayer's phone when ticked", async () => {
    const { d, sent } = deps({ chat_key: "905321234567", link: null, suggestions: [] }, [], atlas, undefined, { savePhone: true });
    const res = await save(d, "expense", chat, pdf);
    expect(sent.map((r) => r.type)).toEqual(["lookup", "link", "savePhone", "capture"]);
    expect(sent[2]).toEqual({ type: "savePhone", engagementUuid: "eng-1", phone: "+905321234567" });
    expect((res as { message: string }).message).toMatch(/Numara mükellefin telefonu olarak kaydedildi\.$/);
  });

  it("still saves the receipt when the phone cannot be written", async () => {
    const { d, sent } = deps({ chat_key: "x", link: null, suggestions: [] }, [], atlas, undefined, { savePhone: true, phoneError: "Yetkiniz yok." });
    const res = await save(d, "expense", chat, pdf);
    expect(sent.at(-1)?.type).toBe("capture");
    expect(res).toMatchObject({ status: "saved" });
    expect((res as { message: string }).message).toMatch(/Numara kaydedilemedi: Yetkiniz yok\.$/);
  });

  it("does not offer to save the number of a group chat", async () => {
    const { d, sent } = deps({ chat_key: "g", link: null, suggestions: [] }, [], atlas, undefined, { savePhone: true });
    await save(d, "expense", { jid: "120363000000000000@g.us", title: "Muhasebe" }, pdf);
    expect(d.pickTaxpayer).toHaveBeenCalledWith("Muhasebe", [], null);
    expect(sent.map((r) => r.type)).not.toContain("savePhone");
  });

  it("warns instead of sending the same receipt twice", async () => {
    const prior: CaptureStatus = {
      sha256: SHA, uuid: "c0", target: "expense", engagement_uuid: "eng-1", taxpayer_title: "Atlas Gıda", state: "pending", created_at: "2026-11-01T10:00:00Z",
    };
    const { d, sent } = deps({ chat_key: "x", link: null, suggestions: [atlas] }, [], atlas, undefined, { statuses: [prior] });
    const res = await save(d, "expense", chat, pdf);
    expect(res).toEqual({ status: "exists", message: "Bu fiş zaten kaydedildi (Atlas Gıda).", existing: prior });
    expect(sent.map((r) => r.type)).toEqual(["statuses"]);
  });

  it("lets the same file go to a different target", async () => {
    const prior: CaptureStatus = {
      sha256: SHA, uuid: "c0", target: "document", engagement_uuid: "eng-1", taxpayer_title: "Atlas Gıda", state: "saved", created_at: "2026-11-01T10:00:00Z",
    };
    const { d, sent } = deps({ chat_key: "x", link: null, suggestions: [atlas] }, [], atlas, undefined, { statuses: [prior] });
    const res = await save(d, "expense", chat, pdf);
    expect(res).toMatchObject({ status: "saved", sha256: SHA });
    expect(sent.map((r) => r.type)).toEqual(["statuses", "lookup", "capture"]);
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
    expect(resultMessage(capture({ replayed: true }))).toBe("Bu dosya zaten kaydedildi (Atlas Gıda).");
    expect(resultMessage(capture({ replayed: true, target: "expense" }))).toBe("Bu fiş zaten kaydedildi (Atlas Gıda).");
    expect(resultMessage(capture({ target: "income", receipt_status: "draft" }))).toMatch(/Z raporu/);
  });
});

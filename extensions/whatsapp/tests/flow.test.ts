import { describe, expect, it, vi } from "vitest";

import { fileNameFor, resultMessage, save, statementSummary, toBase64, type FlowDeps } from "../src/flow";
import type { BankStatementResult, CashBox, Capture, ChatLookup, OpenTask, Reply, Request, Taxpayer } from "../src/messages";

const atlas: Taxpayer = { engagement_uuid: "eng-1", taxpayer_uuid: "tp-1", title: "Atlas Gıda", tax_id: "9000000017" };
const chat = { jid: "905321234567@c.us", title: "Okan Atlas" };
const pdf = { blob: new Blob(["%PDF-1.4"], { type: "application/pdf" }), name: "kira.pdf" };

function capture(over: Partial<Capture> = {}): Capture {
  return {
    uuid: "c1", target: "document", file_name: "kira.pdf", taxpayer: atlas, receipt_uuid: null, receipt_status: null,
    task_uuid: null, task_title: null, has_file: true, replayed: false, ...over,
  };
}

function deps(
  lookup: ChatLookup,
  tasks: OpenTask[] = [],
  picked: Taxpayer | null = atlas,
  task?: OpenTask | null,
  boxes: CashBox[] = [],
  box?: CashBox | null,
) {
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
        case "cashBoxes":
          return { ok: true, data: boxes as T };
        case "capture":
          return { ok: true, data: capture({ target: req.target, task_uuid: req.taskUuid ?? null, task_title: req.taskUuid ? "Kira sözleşmesi" : null }) as T };
        default:
          return { ok: false, error: "unexpected" };
      }
    },
    pickTaxpayer: vi.fn(async () => (picked ? { taxpayer: picked, remember: true } : null)),
    pickTask: vi.fn(async () => task),
    pickCashBox: vi.fn(async () => box),
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
      pickCashBox: vi.fn(),
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
});

describe("bank statement (#780)", () => {
  const linked: ChatLookup = { chat_key: "x", link: { uuid: "l", chat_key: "x", label: "", taxpayer: atlas }, suggestions: [] };
  const csv = { blob: new Blob(["Tarih;Açıklama;Tutar;Bakiye\n"], { type: "text/csv" }), name: "ekstre.csv" };
  const akbank: CashBox = { uuid: "box-1", type: "bank_account", name: "Akbank TL", bank_id: "akbank", iban: null, currency: "TRY", active: true };
  const result = (over: Partial<BankStatementResult> = {}): BankStatementResult => ({
    import_id: "imp-1", format: "csv", bank_id: "", accounts: 1, lines: 3, inserted: 3, merged: 0, skipped: 0, possible_duplicates: 0, gaps: false, ...over,
  });

  it("asks for the bank account and sends it with the file", async () => {
    const { d, sent } = deps(linked, [], atlas, undefined, [akbank], akbank);
    const res = await save(d, "bank_statement", chat, csv);
    expect(d.pickCashBox).toHaveBeenCalledWith("Atlas Gıda", [akbank]);
    expect(sent.map((r) => r.type)).toEqual(["lookup", "cashBoxes", "capture"]);
    expect(sent.at(-1)).toMatchObject({ type: "capture", target: "bank_statement", cashBoxUuid: "box-1", fileName: "ekstre.csv", mimeType: "text/csv" });
    expect(res.status).toBe("saved");
  });

  it("lets the file pick the account, and stops on cancel", async () => {
    let { d, sent } = deps(linked, [], atlas, undefined, [akbank], undefined);
    await save(d, "bank_statement", chat, csv);
    expect(sent.at(-1)).not.toHaveProperty("cashBoxUuid");

    ({ d, sent } = deps(linked, [], atlas, undefined, [akbank], null));
    expect(await save(d, "bank_statement", chat, csv)).toEqual({ status: "cancelled" });
    expect(sent.map((r) => r.type)).toEqual(["lookup", "cashBoxes"]);
  });

  it("does not ask without accounts", async () => {
    const { d, sent } = deps(linked, [], atlas, undefined, []);
    await save(d, "bank_statement", chat, csv);
    expect(d.pickCashBox).not.toHaveBeenCalled();
    expect(sent.map((r) => r.type)).toEqual(["lookup", "cashBoxes", "capture"]);
  });

  it("says how many rows were imported", () => {
    const c = (over: Partial<BankStatementResult>, replayed = false) =>
      capture({ target: "bank_statement", has_file: false, replayed, bank_statement: result(over) });
    expect(resultMessage(c({}))).toBe("Atlas Gıda: 3 satır içe aktarıldı.");
    expect(statementSummary(c({ inserted: 2, skipped: 1 }))).toBe("2 satır içe aktarıldı. 1 satır zaten kayıtlıydı.");
    expect(statementSummary(c({ inserted: 0, skipped: 3 }, true))).toBe("yeni satır yok, 3 satırın hepsi zaten kayıtlı.");
    expect(statementSummary(c({ gaps: true }))).toMatch(/Bakiye tutarsızlığı/);
    expect(statementSummary(c({ lines: 0, inserted: 0 }))).toBe("ekstrede hareket yok.");
  });
});

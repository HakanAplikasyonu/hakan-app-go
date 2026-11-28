// Bir düğme tıklamasının akışı: dosyayı al → mükellefi bul (ya da seçtir)
// → belgede açık talep varsa sor → arka plana gönder → sonucu bildir.
// Tarayıcı API'leri (runtime mesajı, sayfa) dışarıdan verilir; testler
// sahteleriyle çalıştırır.

import type { ChatInfo } from "./dom";
import type { Capture, CashBox, ChatLookup, OpenTask, Reply, Request, Target, Taxpayer } from "./messages";

export interface FileData {
  blob: Blob;
  name: string;
}

export interface FlowDeps {
  send<T>(req: Request): Promise<Reply<T>>;
  pickTaxpayer(chatTitle: string, suggestions: Taxpayer[]): Promise<{ taxpayer: Taxpayer; remember: boolean } | null>;
  pickTask(taxpayerTitle: string, tasks: OpenTask[]): Promise<OpenTask | undefined | null>;
  /** Ekstre için banka hesabı: undefined = dosyadan bulunsun, null = iptal. */
  pickCashBox(taxpayerTitle: string, boxes: CashBox[]): Promise<CashBox | undefined | null>;
  /** Seçimler bitti, dosya gönderilmek üzere (bildirim için). */
  onSending?(): void;
}

export type FlowResult = { status: "saved"; capture: Capture; message: string } | { status: "cancelled" } | { status: "error"; message: string };

class FlowError extends Error {}

async function call<T>(deps: FlowDeps, req: Request): Promise<T> {
  const r = await deps.send<T>(req);
  if (!r.ok) throw new FlowError(r.error);
  return r.data;
}

export async function toBase64(blob: Blob): Promise<string> {
  const bytes = new Uint8Array(await blob.arrayBuffer());
  let bin = "";
  const chunk = 0x8000;
  for (let i = 0; i < bytes.length; i += chunk) {
    bin += String.fromCharCode(...bytes.subarray(i, i + chunk));
  }
  return btoa(bin);
}

const EXT_BY_TYPE: Record<string, string> = {
  "image/jpeg": ".jpg",
  "image/png": ".png",
  "image/webp": ".webp",
  "application/pdf": ".pdf",
  "text/csv": ".csv",
  "application/vnd.ms-excel": ".xls",
  "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet": ".xlsx",
};

export function fileNameFor(file: FileData, chat: ChatInfo): string {
  if (file.name) return file.name;
  const ext = EXT_BY_TYPE[file.blob.type] ?? "";
  const day = new Date().toISOString().slice(0, 10);
  return `whatsapp-${chat.jid.split("@")[0]}-${day}${ext}`;
}

export function resultMessage(c: Capture): string {
  const who = c.taxpayer.title;
  if (c.replayed && c.target !== "bank_statement") return `${who}: bu dosya daha önce kaydedilmiş.`;
  switch (c.target) {
    case "expense":
      return c.receipt_status === "duplicate" ? `${who}: fiş daha önce kayıtlı (mükerrer).` : `${who}: gider taslağı oluşturuldu, onay bekliyor.`;
    case "income":
      return c.receipt_status === "duplicate" ? `${who}: Z raporu daha önce kayıtlı (mükerrer).` : `${who}: gelir (Z raporu) taslağı oluşturuldu, onay bekliyor.`;
    case "document":
      return c.task_title ? `${who}: belge “${c.task_title}” talebine eklendi.` : `${who}: belge kaydedildi.`;
    case "foreign_invoice":
      return `${who}: yabancı hizmet faturası taslağı oluşturuldu, ofiste incelenecek.`;
    case "bank_statement":
      return `${who}: ${statementSummary(c)}`;
  }
}

/** Ekstre sonucu, şerit rozetinde de gösterilir: "12 satır içe aktarıldı". */
export function statementSummary(c: Capture): string {
  const s = c.bank_statement;
  if (!s) return "ekstre içe aktarıldı.";
  if (s.lines === 0) return "ekstrede hareket yok.";
  if (s.inserted === 0) return `yeni satır yok, ${s.lines} satırın hepsi zaten kayıtlı.`;
  let out = `${s.inserted} satır içe aktarıldı.`;
  const known = s.skipped + s.merged;
  if (known > 0) out += ` ${known} satır zaten kayıtlıydı.`;
  if (s.gaps) out += " Bakiye tutarsızlığı var, panelde kontrol edin.";
  return out;
}

/** Dosyası alınmış bir kaydı tamamlar. */
export async function save(deps: FlowDeps, target: Target, chat: ChatInfo, file: FileData): Promise<FlowResult> {
  try {
    const lookup = await call<ChatLookup>(deps, { type: "lookup", chatKey: chat.jid });
    let taxpayer = lookup.link?.taxpayer ?? null;
    if (!taxpayer) {
      const picked = await deps.pickTaxpayer(chat.title, lookup.suggestions);
      if (!picked) return { status: "cancelled" };
      taxpayer = picked.taxpayer;
      if (picked.remember) {
        await call<ChatLookup>(deps, { type: "link", engagementUuid: taxpayer.engagement_uuid, chatKey: chat.jid, label: chat.title });
      }
    }
    let taskUuid: string | undefined;
    if (target === "document") {
      const tasks = await call<OpenTask[]>(deps, { type: "tasks", engagementUuid: taxpayer.engagement_uuid }).catch(() => []);
      if (tasks.length > 0) {
        const t = await deps.pickTask(taxpayer.title, tasks);
        if (t === null) return { status: "cancelled" };
        taskUuid = t?.uuid;
      }
    }
    let cashBoxUuid: string | undefined;
    if (target === "bank_statement") {
      // Hesap listesi okunamazsa (yetki) dosyadan bulunması denenir.
      const boxes = await call<CashBox[]>(deps, { type: "cashBoxes", engagementUuid: taxpayer.engagement_uuid }).catch(() => []);
      if (boxes.length > 0) {
        const b = await deps.pickCashBox(taxpayer.title, boxes);
        if (b === null) return { status: "cancelled" };
        cashBoxUuid = b?.uuid;
      }
    }
    deps.onSending?.();
    const capture = await call<Capture>(deps, {
      type: "capture",
      engagementUuid: taxpayer.engagement_uuid,
      target,
      chatKey: chat.jid,
      fileName: fileNameFor(file, chat),
      mimeType: file.blob.type || "application/octet-stream",
      data: await toBase64(file.blob),
      ...(taskUuid ? { taskUuid } : {}),
      ...(cashBoxUuid ? { cashBoxUuid } : {}),
    });
    return { status: "saved", capture, message: resultMessage(capture) };
  } catch (e) {
    return { status: "error", message: e instanceof Error ? e.message : "İşlem tamamlanamadı." };
  }
}

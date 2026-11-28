// Bir düğme tıklamasının akışı: dosyayı al → mükellefi bul (ya da seçtir)
// → belgede açık talep varsa sor → arka plana gönder → sonucu bildir.
// Tarayıcı API'leri (runtime mesajı, sayfa) dışarıdan verilir; testler
// sahteleriyle çalıştırır.

import type { ChatInfo } from "./dom";
import { alreadySaved, latestFor, phoneOfChat, type CaptureStatus } from "./badge";
import type { Capture, ChatLookup, OpenTask, Reply, Request, Target, Taxpayer } from "./messages";

export interface FileData {
  blob: Blob;
  name: string;
}

export interface FlowDeps {
  send<T>(req: Request): Promise<Reply<T>>;
  /** phone: sohbetin numarası (kişi sohbetiyse); seçici "telefon olarak kaydet" kutusunu gösterir. */
  pickTaxpayer(
    chatTitle: string,
    suggestions: Taxpayer[],
    phone: string | null,
  ): Promise<{ taxpayer: Taxpayer; remember: boolean; savePhone?: boolean } | null>;
  pickTask(taxpayerTitle: string, tasks: OpenTask[]): Promise<OpenTask | undefined | null>;
  /** Seçimler bitti, dosya gönderilmek üzere (bildirim için). */
  onSending?(): void;
  /** Dosya özeti (SHA-256): verilirse aynı dosya aynı türe yeniden gönderilmez. */
  hash?(blob: Blob): Promise<string>;
}

export type FlowResult =
  | { status: "saved"; capture: Capture; message: string; autoMatched?: boolean; sha256?: string }
  /** Aynı medya bu türe daha önce kaydedilmiş: hiçbir şey gönderilmedi. */
  | { status: "exists"; message: string; existing: CaptureStatus }
  | { status: "cancelled" }
  | { status: "error"; message: string };

/** Aynı dosya aynı türe kaydedildiyse uyarı; özet ya da sorgu başarısızsa kayıt sürer. */
async function existing(deps: FlowDeps, target: Target, blob: Blob): Promise<{ sha?: string; hit?: FlowResult }> {
  if (!deps.hash) return {};
  let sha: string;
  try {
    sha = await deps.hash(blob);
  } catch {
    return {};
  }
  const r = await deps.send<CaptureStatus[]>({ type: "statuses", sha256s: [sha] });
  if (!r.ok) return { sha };
  const warn = alreadySaved(r.data, sha, target);
  const hit = latestFor(r.data.filter((x) => x.target === target), sha);
  return warn && hit ? { sha, hit: { status: "exists", message: warn, existing: hit } } : { sha };
}

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
};

export function fileNameFor(file: FileData, chat: ChatInfo): string {
  if (file.name) return file.name;
  const ext = EXT_BY_TYPE[file.blob.type] ?? "";
  const day = new Date().toISOString().slice(0, 10);
  return `whatsapp-${chat.jid.split("@")[0]}-${day}${ext}`;
}

export function resultMessage(c: Capture): string {
  const who = c.taxpayer.title;
  if (c.replayed) {
    return c.target === "expense" || c.target === "income"
      ? `Bu fiş zaten kaydedildi (${who}).`
      : `Bu dosya zaten kaydedildi (${who}).`;
  }
  switch (c.target) {
    case "expense":
      return c.receipt_status === "duplicate" ? `${who}: fiş daha önce kayıtlı (mükerrer).` : `${who}: gider taslağı oluşturuldu, onay bekliyor.`;
    case "income":
      return c.receipt_status === "duplicate" ? `${who}: Z raporu daha önce kayıtlı (mükerrer).` : `${who}: gelir (Z raporu) taslağı oluşturuldu, onay bekliyor.`;
    case "document":
      return c.task_title ? `${who}: belge “${c.task_title}” talebine eklendi.` : `${who}: belge kaydedildi.`;
    case "foreign_invoice":
      return `${who}: yabancı hizmet faturası taslağı oluşturuldu, ofiste incelenecek.`;
  }
}

/** Dosyası alınmış bir kaydı tamamlar. */
export async function save(deps: FlowDeps, target: Target, chat: ChatInfo, file: FileData): Promise<FlowResult> {
  try {
    const pre = await existing(deps, target, file.blob);
    if (pre.hit) return pre.hit;
    const lookup = await call<ChatLookup>(deps, { type: "lookup", chatKey: chat.jid });
    let taxpayer = lookup.link?.taxpayer ?? null;
    let autoMatched = false;
    let note = "";
    if (!taxpayer && lookup.suggestions.length === 1) {
      // Sohbet numarası tek bir mükellefin telefonuyla eşleşti: sorulmaz (#780).
      taxpayer = lookup.suggestions[0]!;
      autoMatched = true;
    }
    if (!taxpayer) {
      const phone = phoneOfChat(chat.jid);
      const picked = await deps.pickTaxpayer(chat.title, lookup.suggestions, phone);
      if (!picked) return { status: "cancelled" };
      taxpayer = picked.taxpayer;
      if (picked.remember) {
        await call<ChatLookup>(deps, { type: "link", engagementUuid: taxpayer.engagement_uuid, chatKey: chat.jid, label: chat.title });
      }
      if (picked.savePhone && phone) {
        // Kayıt sürsün: telefon yazılamazsa (yetki, geçersiz numara) yalnızca bildirilir.
        const r = await deps.send<boolean>({ type: "savePhone", engagementUuid: taxpayer.engagement_uuid, phone });
        note = r.ok ? " Numara mükellefin telefonu olarak kaydedildi." : ` Numara kaydedilemedi: ${r.error}`;
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
    });
    const prefix = autoMatched ? `Mükellef telefondan bulundu: ${taxpayer.title}. ` : "";
    return { status: "saved", capture, message: prefix + resultMessage(capture) + note, autoMatched, ...(pre.sha ? { sha256: pre.sha } : {}) };
  } catch (e) {
    return { status: "error", message: e instanceof Error ? e.message : "İşlem tamamlanamadı." };
  }
}

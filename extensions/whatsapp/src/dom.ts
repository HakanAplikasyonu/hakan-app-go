// Sayfadan okunanlar: sohbet kimliği (chat.ts ya da eski data-id), sohbet başlığı ve kaydedilebilir
// medya. Seçicilerin tamamı selectors.ts'tedir. Mesaj metni okunmaz:
// fonksiyonlar textContent / innerText kullanmaz, yalnızca öznitelik okur.

import { requestChat, type PageChat } from "./chat";
import { CHAT_ID_RE, DOCUMENT_EXTENSIONS, SELECTORS, closestAny, queryFirst } from "./selectors";

export interface ChatInfo {
  /** WhatsApp sohbet kimliği (905551112233@c.us, …@g.us, …@lid). */
  jid: string;
  /** Sohbetin adı (başlıktaki title); eşleme listesinde gösterilir. */
  title: string;
}

export type MediaKind = "image" | "document";

export interface MediaItem {
  /** Düğmelerin ekleneceği mesaj satırı. */
  row: Element;
  kind: MediaKind;
  /** image: tam görselin blob adresi; null = henüz indirilmedi. */
  src: string | null;
  /** document: dosya adı (title özniteliği). */
  fileName: string;
  /** Gider / gelir yalnızca görselde (fiş okuma görselle çalışır). */
  receiptable: boolean;
}

export function parseChatJid(dataId: string | null | undefined): string | null {
  if (!dataId) return null;
  const m = CHAT_ID_RE.exec(dataId);
  return m?.[1] ?? null;
}

export function chatTitle(doc: Document): string {
  const el = queryFirst<HTMLElement>(doc, SELECTORS.chatTitle);
  // title özniteliği sohbet adıdır; yoksa yalnızca başlık öğesinin kendi
  // adı (mesaj içeriği değil) alınır.
  const raw = el?.getAttribute("title") ?? el?.getAttribute("aria-label") ?? "";
  return raw.trim().slice(0, 120);
}

const ROW_SELECTOR = SELECTORS.messageRow.join(", ");

/** Açık sohbetin mesaj satırları (iç içe eşleşmeler ve gruplanmış çıkartmalar hariç). */
export function messageRows(doc: Document): Element[] {
  const out: Element[] = [];
  for (const row of Array.from(doc.querySelectorAll(ROW_SELECTOR))) {
    const id = row.getAttribute("data-id") ?? "";
    if (SELECTORS.messageRowSkipPrefix.some((p) => id.startsWith(p))) continue;
    if (row.parentElement?.closest(ROW_SELECTOR)) continue;
    out.push(row);
  }
  return out;
}

/** Eski sürüm: kimlik herhangi bir mesaj satırının data-id'sinden gelir. */
export function legacyChat(doc: Document): ChatInfo | null {
  for (const row of Array.from(doc.querySelectorAll(SELECTORS.legacyMessageRow))) {
    const jid = parseChatJid(row.getAttribute("data-id"));
    if (jid) return { jid, title: chatTitle(doc) };
  }
  return null;
}

/**
 * Açık sohbet. Önce sayfanın kendi dünyasına sorulur (güncel WhatsApp,
 * chat.ts); yanıt yoksa eski data-id ayrıştırmasına düşülür.
 */
export async function currentChat(
  doc: Document,
  ask: (win: Window) => Promise<PageChat | null> = requestChat,
): Promise<ChatInfo | null> {
  const win = doc.defaultView;
  let page: PageChat | null = null;
  if (win) {
    try {
      page = await ask(win);
    } catch {
      page = null;
    }
  }
  if (page?.jid) return { jid: page.jid, title: page.title || chatTitle(doc) };
  return legacyChat(doc);
}

function isExcludedImage(img: HTMLImageElement): boolean {
  if (closestAny(img, SELECTORS.imageExcluded)) return true;
  const w = Number(img.getAttribute("width") ?? "0");
  const h = Number(img.getAttribute("height") ?? "0");
  // Emoji ve küçük simgeler.
  return (w > 0 && w < 64) || (h > 0 && h < 64);
}

function documentFileName(row: Element): string {
  const named = queryFirst<HTMLElement>(row, SELECTORS.documentName);
  return (named?.getAttribute("title") ?? "").trim();
}

function hasDocumentExtension(name: string): boolean {
  const lower = name.toLowerCase();
  return DOCUMENT_EXTENSIONS.some((ext) => lower.endsWith(ext));
}

/** Bir mesaj satırındaki kaydedilebilir medya (yoksa null). */
export function mediaOf(row: Element): MediaItem | null {
  if (queryFirst(row, SELECTORS.documentBubble)) {
    const fileName = documentFileName(row);
    if (!fileName || !hasDocumentExtension(fileName)) return null;
    const lower = fileName.toLowerCase();
    return { row, kind: "document", src: null, fileName, receiptable: !lower.endsWith(".pdf") && !lower.endsWith(".heic") };
  }
  const img = Array.from(row.querySelectorAll<HTMLImageElement>(SELECTORS.messageImage)).find((i) => !isExcludedImage(i));
  if (img) return { row, kind: "image", src: img.getAttribute("src"), fileName: "", receiptable: true };
  const preview = Array.from(row.querySelectorAll<HTMLImageElement>(SELECTORS.messageImagePreview)).find((i) => !isExcludedImage(i));
  if (preview) return { row, kind: "image", src: null, fileName: "", receiptable: true };
  return null;
}

/** Açık sohbetteki bütün medya mesajları. */
export function findMedia(doc: Document): MediaItem[] {
  const out: MediaItem[] = [];
  for (const row of messageRows(doc)) {
    const m = mediaOf(row);
    if (m) out.push(m);
  }
  return out;
}

export interface ViewerItem {
  viewer: Element;
  toolbar: Element;
  src: string;
}

/** Açık görsel görüntüleyici (tam çözünürlüklü görsel). */
export function findViewer(doc: Document): ViewerItem | null {
  const viewer = queryFirst(doc, SELECTORS.mediaViewer);
  if (!viewer) return null;
  const img = viewer.querySelector<HTMLImageElement>(SELECTORS.mediaViewerImage);
  const src = img?.getAttribute("src");
  if (!src) return null;
  const toolbar = queryFirst(viewer, SELECTORS.mediaViewerToolbar) ?? viewer;
  return { viewer, toolbar, src };
}

/** Belgenin indirme denetimi (uzantı tıklatıp dosyayı yakalar). */
export function documentDownloadControl(row: Element): HTMLElement | null {
  const hit = queryFirst<HTMLElement>(row, SELECTORS.documentDownload);
  if (hit) return (hit.closest("button, [role='button']") as HTMLElement | null) ?? hit;
  return queryFirst<HTMLElement>(row, SELECTORS.documentBubble);
}

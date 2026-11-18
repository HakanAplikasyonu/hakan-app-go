// Aktif sohbetin kimliği, sayfanın kendi dünyasından (world: MAIN).
//
// Güncel WhatsApp Web mesaj satırının data-id'sine sohbet kimliğini artık
// yazmıyor (yalnızca mesaj kimliği kalıyor). Kimlik, WhatsApp'ın kendi modül
// sisteminden okunur: MAIN dünyasındaki yanıtlayıcı (main-world.ts) aktif
// sohbetin kimliğini ve adını verir; içerik betiği postMessage ile sorar.
//
// Gizlilik: yalnızca aktif sohbet nesnesinin kimliği ve adı okunur. Mesaj
// listesine, mesaj içeriğine ya da başka sohbetlere dokunulmaz.

export const CHAT_ASK = "hakan-wa:chat?";
export const CHAT_REPLY = "hakan-wa:chat";

/** Yanıt beklenecek en uzun süre (MAIN dünyası yoksa geri dönüşe geçilir). */
export const CHAT_MS = 1000;

export interface PageChat {
  jid: string;
  title: string;
}

type AnyObj = Record<string, unknown> | null | undefined;
type Win = Window & typeof globalThis;

const JID_RE = /^[0-9A-Za-z-]+@(?:c\.us|g\.us|lid|s\.whatsapp\.net)$/;

function obj(v: unknown): AnyObj {
  return v && (typeof v === "object" || typeof v === "function") ? (v as Record<string, unknown>) : null;
}

/** Wid nesnesi ya da dize → "905551112233@c.us" (geçersizse null). */
export function serializeWid(v: unknown): string | null {
  let s: unknown = v;
  const o = obj(v);
  if (o) {
    s = o._serialized;
    if (typeof s !== "string" && typeof o.user === "string" && typeof o.server === "string") s = `${o.user}@${o.server}`;
  }
  if (typeof s !== "string") return null;
  s = s.trim();
  return JID_RE.test(s as string) ? (s as string) : null;
}

/** Kişi sohbetinde telefon numaralı kimlik (backend telefonla öneri yapabilsin). */
function phoneJid(chat: Record<string, unknown>): string | null {
  const contact = obj(chat.contact);
  for (const v of [contact?.phoneNumber, chat.phoneNumber, contact?.pn, obj(chat.id)?.pn]) {
    let jid = serializeWid(v);
    if (!jid && typeof v === "string" && /^\d{6,20}$/.test(v)) jid = `${v}@c.us`;
    if (jid?.endsWith("@s.whatsapp.net")) jid = jid.replace(/@s\.whatsapp\.net$/, "@c.us");
    if (jid?.endsWith("@c.us")) return jid;
  }
  return null;
}

/** WhatsApp sohbet nesnesinden kimlik ve ad. */
export function chatFromModel(model: unknown): PageChat | null {
  const chat = obj(model);
  if (!chat) return null;
  const id = serializeWid(chat.id);
  if (!id) return null;
  const jid = id.endsWith("@g.us") ? id : (phoneJid(chat) ?? id);
  let title = "";
  for (const t of [chat.formattedTitle, chat.name, obj(chat.contact)?.name]) {
    if (typeof t === "string" && t.trim()) {
      title = t.trim().slice(0, 120);
      break;
    }
  }
  return { jid, title };
}

function activeOf(collection: unknown): unknown {
  const c = obj(collection);
  const getActive = c?.getActive;
  return typeof getActive === "function" ? getActive.call(c) : null;
}

/** Sırayla denenen aktif sohbet erişimcileri (WhatsApp sürümüne göre). */
const ACCESSORS: Array<(w: Win) => unknown> = [
  (w) => {
    const req = (w as unknown as { require?: (m: string) => unknown }).require;
    return typeof req === "function" ? activeOf(obj(req("WAWebChatCollection"))?.ChatCollection) : null;
  },
  (w) => {
    const req = (w as unknown as { require?: (m: string) => unknown }).require;
    return typeof req === "function" ? activeOf(obj(req("WAWebCollections"))?.Chat) : null;
  },
  // Eski Store yolu: başka bir araç window.Store'u açmışsa.
  (w) => activeOf(obj((w as unknown as { Store?: unknown }).Store)?.Chat),
  // Eski webpack yolu: yalnızca önceden yüklenmiş modüllerin önbelleği gezilir
  // (yeni modül çalıştırılmaz).
  (w) => {
    const chunks = (w as unknown as { webpackChunkwhatsapp_web_client?: unknown[] }).webpackChunkwhatsapp_web_client;
    if (!Array.isArray(chunks)) return null;
    let req: { c?: Record<string, { exports?: unknown }> } | null = null;
    chunks.push([[`hakan-wa-${Date.now()}`], {}, (r: typeof req) => (req = r)]);
    const cache = (req as { c?: Record<string, { exports?: unknown }> } | null)?.c;
    if (!cache) return null;
    for (const key of Object.keys(cache)) {
      const exp = obj(cache[key]?.exports);
      for (const cand of [obj(exp?.default)?.Chat, exp?.Chat, exp?.ChatCollection]) {
        const c = obj(cand);
        if (c && typeof c.getActive === "function" && typeof c.get === "function") return activeOf(c);
      }
    }
    return null;
  },
];

/** MAIN dünyası: aktif sohbet (bulunamazsa null). */
export function readActiveChat(win: Win): PageChat | null {
  for (const accessor of ACCESSORS) {
    try {
      const chat = chatFromModel(accessor(win));
      if (chat) return chat;
    } catch {
      // Bu erişimci bu sürümde yok; sıradakine geç.
    }
  }
  return null;
}

/** MAIN dünyası: içerik betiğinin sorularını yanıtlar. */
export function installChatResponder(win: Win, read: (w: Win) => PageChat | null = readActiveChat): void {
  const w = win as Win & { __hakanWaChat?: boolean };
  if (w.__hakanWaChat) return;
  w.__hakanWaChat = true;
  win.addEventListener("message", (ev: MessageEvent) => {
    if (ev.source !== win || ev.origin !== win.location.origin) return;
    if (ev.data?.type !== CHAT_ASK || typeof ev.data.token !== "string") return;
    let chat: PageChat | null = null;
    try {
      chat = read(win);
    } catch {
      chat = null;
    }
    win.postMessage({ type: CHAT_REPLY, token: ev.data.token, jid: chat?.jid ?? null, title: chat?.title ?? "" }, win.location.origin);
  });
}

/** İçerik betiği: aktif sohbeti MAIN dünyasından ister (yanıt yoksa null). */
export function requestChat(win: Window, timeoutMs = CHAT_MS): Promise<PageChat | null> {
  const token = Math.random().toString(36).slice(2) + Date.now().toString(36);
  return new Promise((resolve) => {
    const done = (v: PageChat | null) => {
      win.clearTimeout(timer);
      win.removeEventListener("message", onMessage);
      resolve(v);
    };
    const onMessage = (ev: MessageEvent) => {
      if (ev.source !== win || ev.origin !== win.location.origin) return;
      if (ev.data?.type !== CHAT_REPLY || ev.data.token !== token) return;
      const jid = serializeWid(ev.data.jid);
      done(jid ? { jid, title: typeof ev.data.title === "string" ? ev.data.title.slice(0, 120) : "" } : null);
    };
    const timer = win.setTimeout(() => done(null), timeoutMs);
    win.addEventListener("message", onMessage);
    win.postMessage({ type: CHAT_ASK, token }, win.location.origin);
  });
}

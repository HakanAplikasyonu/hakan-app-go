// Arka plan hizmet çalışanı: API çağrılarının tek yeri. İçerik betiği
// web.whatsapp.com kökeninde çalıştığı için API'ye doğrudan istek atmaz
// (CORS); dosya ve seçimleri buraya mesajla gönderir.

import { Api, describeError, type KV } from "./api";
import type { Reply, Request } from "./messages";
import { badgeText, badgeTitle, POLL_MS, type Summary } from "./summary";

function area(a: chrome.storage.StorageArea): KV {
  return {
    async get(key) {
      const v = await a.get(key);
      return v[key];
    },
    set: (key, value) => a.set({ [key]: value }),
    remove: (key) => a.remove(key),
  };
}

const sessionKV = area(chrome.storage.session);
const api = new Api(area(chrome.storage.local), sessionKV);

const TITLE = "Easytrade Müşavirim Ofis · WhatsApp";

async function paintBadge(s: (Summary & { webBase: string }) | null): Promise<void> {
  const counts = s?.inbox?.counts ?? null;
  await chrome.action.setBadgeText({ text: badgeText(counts?.total) });
  await chrome.action.setBadgeBackgroundColor({ color: "#2448FF" });
  await chrome.action.setTitle({ title: badgeTitle(TITLE, counts) });
}

let inflight: Promise<unknown> | null = null;

/**
 * Ofis özeti ve rozet. "alarms" izni olmadığından zamanlayıcı yok: WhatsApp
 * Web sekmesi açıkken içerik betiği 5 dakikada bir ister, açılır pencere
 * açılınca da istenir. 5 dakikadan yeni önbellek varsa sunucuya gidilmez.
 */
async function summary(force = false): Promise<unknown> {
  const cached = (await sessionKV.get("summary")) as (Summary & { webBase: string }) | undefined;
  if (!force && cached && Date.now() - cached.fetchedAt < POLL_MS) return cached;
  if (!inflight) {
    inflight = (async () => {
      const status = await api.status();
      if (!status.paired) {
        await sessionKV.remove("summary");
        await paintBadge(null);
        return null;
      }
      try {
        const s = await api.summary();
        await sessionKV.set("summary", s);
        await paintBadge(s);
        return s;
      } catch (e) {
        await paintBadge(null);
        throw e;
      }
    })().finally(() => {
      inflight = null;
    });
  }
  return inflight;
}

function fromBase64(data: string, type: string): Blob {
  const bin = atob(data);
  const bytes = new Uint8Array(bin.length);
  for (let i = 0; i < bin.length; i++) bytes[i] = bin.charCodeAt(i);
  return new Blob([bytes], { type });
}

export async function handle(req: Request): Promise<unknown> {
  switch (req.type) {
    case "status":
      return api.status();
    case "pair": {
      const st = await api.pair(req.apiBase, req.code, req.deviceName);
      void summary(true).catch(() => undefined);
      return st;
    }
    case "unpair":
      await api.unpair();
      await sessionKV.remove("summary");
      await paintBadge(null);
      return api.status();
    case "test":
      await api.ping();
      return true;
    case "lookup":
      return api.lookup(req.chatKey);
    case "search":
      return api.search(req.q);
    case "link":
      return api.link(req.engagementUuid, req.chatKey, req.label);
    case "tasks":
      return api.openTasks(req.engagementUuid);
    case "summary":
      return summary(req.force);
    case "setWebBase": {
      await api.setWebBase(req.webBase);
      return summary(true);
    }
    case "capture":
      return api.capture(req.engagementUuid, req.target, req.chatKey, fromBase64(req.data, req.mimeType), req.fileName, req.taskUuid);
  }
}

chrome.runtime.onStartup.addListener(() => void summary().catch(() => undefined));
chrome.runtime.onInstalled.addListener(() => void summary().catch(() => undefined));

chrome.runtime.onMessage.addListener((req: Request, sender, sendResponse: (r: Reply) => void) => {
  // Yalnızca bu uzantının kendi sayfaları ve içerik betiği.
  if (sender.id !== chrome.runtime.id) return false;
  handle(req).then(
    (data) => sendResponse({ ok: true, data }),
    (e: unknown) => sendResponse({ ok: false, error: describeError(e).message, code: describeError(e).code }),
  );
  return true;
});

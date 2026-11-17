// Arka plan hizmet çalışanı: API çağrılarının tek yeri. İçerik betiği
// web.whatsapp.com kökeninde çalıştığı için API'ye doğrudan istek atmaz
// (CORS); dosya ve seçimleri buraya mesajla gönderir.

import { Api, describeError, type KV } from "./api";
import type { Reply, Request } from "./messages";

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

const api = new Api(area(chrome.storage.local), area(chrome.storage.session));

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
    case "pair":
      return api.pair(req.apiBase, req.code, req.deviceName);
    case "unpair":
      await api.unpair();
      return api.status();
    case "lookup":
      return api.lookup(req.chatKey);
    case "search":
      return api.search(req.q);
    case "link":
      return api.link(req.engagementUuid, req.chatKey, req.label);
    case "tasks":
      return api.openTasks(req.engagementUuid);
    case "capture":
      return api.capture(req.engagementUuid, req.target, req.chatKey, fromBase64(req.data, req.mimeType), req.fileName, req.taskUuid);
  }
}

chrome.runtime.onMessage.addListener((req: Request, sender, sendResponse: (r: Reply) => void) => {
  // Yalnızca bu uzantının kendi sayfaları ve içerik betiği.
  if (sender.id !== chrome.runtime.id) return false;
  handle(req).then(
    (data) => sendResponse({ ok: true, data }),
    (e: unknown) => sendResponse({ ok: false, error: describeError(e).message, code: describeError(e).code }),
  );
  return true;
});

// İçerik betiği (web.whatsapp.com, izole dünya). Medya mesajlarına ve görsel
// görüntüleyiciye "Gider / Gelir / Belge olarak kaydet" düğmelerini ekler.
// Yalnızca kullanıcının tıkladığı dosya gönderilir; mesaj metni okunmaz.

import { isSettled, latestFor, sha256Hex, stateFromCapture, type BadgeState, type CaptureStatus } from "./badge";
import { currentChat, documentDownloadControl, findMedia, findViewer, mediaOf, type MediaItem } from "./dom";
import { save, type FileData, type FlowDeps } from "./flow";
import { captureDownload } from "./intercept";
import type { Reply, Request, Status, Target, Taxpayer } from "./messages";
import { OWN_PREFIX } from "./selectors";
import { POLL_MS } from "./summary";
import { attachBar, badgeOf, pickTask, pickTaxpayer, setBadge, setBarBusy, toast } from "./ui";

const send = <T>(req: Request) => chrome.runtime.sendMessage<Request, Reply<T>>(req);

const baseDeps: FlowDeps = {
  send,
  hash: sha256Hex,
  pickTaxpayer: (chatTitle, suggestions, phone) =>
    pickTaxpayer(document, {
      chatTitle,
      suggestions,
      phone,
      search: async (q) => {
        const r = await send<Taxpayer[]>({ type: "search", q });
        if (!r.ok) throw new Error(r.error);
        return r.data;
      },
    }),
  pickTask: (title, tasks) => pickTask(document, title, tasks),
  onSending: () => toast(document, "Gönderiliyor…", "info", 30000),
};

async function fileOf(item: MediaItem): Promise<FileData> {
  if (item.kind === "image") {
    if (!item.src) throw new Error("Görsel henüz indirilmedi. Önce görsele bir kez tıklayın, sonra yeniden deneyin.");
    const blob = await (await fetch(item.src)).blob();
    return { blob, name: "" };
  }
  const control = documentDownloadControl(item.row);
  if (!control) throw new Error("Belgenin indirme düğmesi bulunamadı.");
  try {
    const got = await captureDownload(window, () => control.click());
    return { blob: got.blob, name: got.name || item.fileName };
  } catch {
    throw new Error("Belge alınamadı. Dosyayı indirip panelden yükleyebilirsiniz.");
  }
}

let running = false;

async function onSave(target: Target, resolve: () => MediaItem | null, bar: Element | null): Promise<void> {
  if (running) return;
  const item = resolve();
  // Kimlik tıklama anında istenir (sayfanın dünyasından, yoksa data-id'den).
  const chat = item ? await currentChat(document) : null;
  if (!chat || !item) {
    toast(document, "Sohbet ya da dosya tanınamadı. Sohbeti yeniden açıp deneyin.", "error");
    return;
  }
  const status = await send<Status>({ type: "status" });
  if (!status.ok || !status.data.paired) {
    toast(document, "Uzantı eşleştirilmemiş. Araç çubuğundaki Easytrade simgesinden, panelde aldığınız kodla bağlayın.", "error", 8000);
    return;
  }
  running = true;
  setBarBusy(bar, true);
  const before = badgeOf(bar);
  const beforeText = bar?.shadowRoot?.querySelector(".badge")?.textContent ?? "";
  const restore = () => {
    if (!before) setBadge(bar, null);
    else if (bar?.shadowRoot) {
      setBadge(bar, before);
      const b = bar.shadowRoot.querySelector(".badge");
      if (b) b.textContent = beforeText;
    }
  };
  // Rozet "Okunuyor…": seçimler bitti, dosya sunucuya gidiyor.
  const deps: FlowDeps = { ...baseDeps, onSending: () => (baseDeps.onSending?.(), setBadge(bar, "reading")) };
  try {
    const file = await fileOf(item);
    const res = await save(deps, target, chat, file);
    document.querySelectorAll(`.${OWN_PREFIX}-toast`).forEach((n) => n.remove());
    if (res.status === "saved") {
      toast(document, res.message, "info", 8000);
      setBadge(bar, stateFromCapture(res.capture.target, res.capture.receipt_status), res.capture.taxpayer.title);
      if (bar) track(bar, res.sha256);
    } else if (res.status === "exists") {
      toast(document, res.message, "error", 8000);
      setBadge(bar, res.existing.state, res.existing.taxpayer_title);
      if (bar) track(bar, res.existing.sha256);
    } else if (res.status === "error") {
      restore();
      toast(document, res.message, "error", 8000);
    } else restore();
  } catch (e) {
    restore();
    toast(document, e instanceof Error ? e.message : "İşlem tamamlanamadı.", "error", 8000);
  } finally {
    running = false;
    setBarBusy(bar, false);
  }
}

// ---------------------------------------------------------------------------
// Rozetler (#780): görünen görsellerin özeti sunucuya sorulur; kaydedilmiş
// olanın altında durumu yazar. Bekleyenler 15 sn'de bir tazelenir.
// ---------------------------------------------------------------------------

const BADGE_POLL_MS = 15_000;
/** Şerit → medyayı yeniden okuyan işlev ve (biliniyorsa) dosya özeti. */
const tracked = new Map<Element, { read: () => MediaItem | null; sha?: string }>();
const shaBySrc = new Map<string, Promise<string | null>>();

function track(host: Element, sha?: string, read?: () => MediaItem | null): void {
  const t = tracked.get(host);
  if (t) {
    if (sha) t.sha = sha;
    return;
  }
  tracked.set(host, { read: read ?? (() => null), ...(sha ? { sha } : {}) });
}

function shaOfSrc(src: string): Promise<string | null> {
  let p = shaBySrc.get(src);
  if (!p) {
    p = fetch(src)
      .then((r) => r.blob())
      .then(sha256Hex)
      .catch(() => null);
    shaBySrc.set(src, p);
  }
  return p;
}

let refreshing = false;
async function refreshBadges(onlyOpen: boolean): Promise<void> {
  if (refreshing) return;
  refreshing = true;
  try {
    const rows: { host: Element; sha: string }[] = [];
    for (const [host, t] of tracked) {
      if (!host.isConnected) {
        tracked.delete(host);
        continue;
      }
      const state = badgeOf(host);
      if (state === "reading" || (onlyOpen && state !== "pending")) continue;
      if (!t.sha) {
        const item = t.read();
        if (item?.kind === "image" && item.src) t.sha = (await shaOfSrc(item.src)) ?? undefined;
      }
      if (t.sha) rows.push({ host, sha: t.sha });
    }
    const shas = [...new Set(rows.map((r) => r.sha))].slice(-50);
    if (shas.length === 0) return;
    const status = await send<Status>({ type: "status" });
    if (!status.ok || !status.data.paired) return;
    const r = await send<CaptureStatus[]>({ type: "statuses", sha256s: shas });
    if (!r.ok) return;
    for (const { host, sha } of rows) {
      if (badgeOf(host) === "reading") continue;
      const hit = latestFor(r.data, sha);
      if (hit) setBadge(host, hit.state, hit.taxpayer_title);
    }
  } finally {
    refreshing = false;
  }
}

let refreshTimer: ReturnType<typeof setTimeout> | undefined;
function scheduleRefresh(): void {
  clearTimeout(refreshTimer);
  refreshTimer = setTimeout(() => void refreshBadges(false), 600);
}

setInterval(() => {
  const open = [...tracked.keys()].some((h) => {
    const s: BadgeState | null = badgeOf(h);
    return s !== null && !isSettled(s);
  });
  if (open) {
    void refreshBadges(true);
    return;
  }
  // Bu arada indirilmiş (önizlemesi gelmiş) görsel varsa onu da sor.
  const fresh = [...tracked.values()].some((t) => {
    if (t.sha) return false;
    const item = t.read();
    return item?.kind === "image" && !!item.src && !shaBySrc.has(item.src);
  });
  if (fresh) void refreshBadges(false);
}, BADGE_POLL_MS);

function scan(): void {
  let added = false;
  for (const item of findMedia(document)) {
    // Tıklamada satır yeniden okunur: görsel bu arada indirilmiş olabilir.
    const read = () => mediaOf(item.row);
    const host: Element | null = attachBar(item, { onSave: (t) => void onSave(t, read, host) });
    if (host) {
      track(host, undefined, read);
      added = true;
    }
  }
  if (added) scheduleRefresh();
  const viewer = findViewer(document);
  if (viewer && !viewer.toolbar.querySelector(`.${OWN_PREFIX}-bar`)) {
    const item: MediaItem = { row: viewer.toolbar, kind: "image", src: viewer.src, fileName: "", receiptable: true };
    const host: Element | null = attachBar(item, {
      onSave: (t) => {
        const v = findViewer(document);
        void onSave(t, () => (v ? { ...item, src: v.src } : null), host);
      },
    });
  }
}

let pending = false;
const observer = new MutationObserver(() => {
  if (pending) return;
  pending = true;
  requestAnimationFrame(() => {
    pending = false;
    scan();
  });
});
observer.observe(document.documentElement, { childList: true, subtree: true });
scan();

// Rozetteki Gelen kutusu sayısı (#780): sekme açıkken 5 dakikada bir tazele.
// Arka plan önbelleği 5 dakikadan yeniyse sunucuya gitmez; hata sessizce yutulur.
const poll = () => void send({ type: "summary" }).catch(() => undefined);
poll();
setInterval(poll, POLL_MS);

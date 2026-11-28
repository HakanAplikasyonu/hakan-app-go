// İçerik betiği (web.whatsapp.com, izole dünya). Medya mesajlarına ve görsel
// görüntüleyiciye "Gider / Gelir / Belge olarak kaydet" düğmelerini ekler.
// Yalnızca kullanıcının tıkladığı dosya gönderilir; mesaj metni okunmaz.

import { currentChat, documentDownloadControl, findMedia, findViewer, mediaOf, type MediaItem } from "./dom";
import { save, statementSummary, type FileData, type FlowDeps } from "./flow";
import { captureDownload } from "./intercept";
import type { Reply, Request, Status, Target, Taxpayer } from "./messages";
import { OWN_PREFIX } from "./selectors";
import { POLL_MS } from "./summary";
import { attachBar, markBar, pickCashBox, pickTask, pickTaxpayer, setBarBusy, toast } from "./ui";

const send = <T>(req: Request) => chrome.runtime.sendMessage<Request, Reply<T>>(req);

const deps: FlowDeps = {
  send,
  pickTaxpayer: (chatTitle, suggestions) =>
    pickTaxpayer(document, {
      chatTitle,
      suggestions,
      search: async (q) => {
        const r = await send<Taxpayer[]>({ type: "search", q });
        if (!r.ok) throw new Error(r.error);
        return r.data;
      },
    }),
  pickTask: (title, tasks) => pickTask(document, title, tasks),
  pickCashBox: (title, boxes) => pickCashBox(document, title, boxes),
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
  try {
    const file = await fileOf(item);
    const res = await save(deps, target, chat, file);
    if (res.status === "saved") {
      toast(document, res.message);
      // Ekstre: sonuç şeritte kalır ("12 satır içe aktarıldı").
      if (target === "bank_statement") markBar(bar, statementSummary(res.capture).replace(/^./, (c) => c.toLocaleUpperCase("tr")));
    } else if (res.status === "error") {
      toast(document, res.message, "error", 8000);
      if (target === "bank_statement") markBar(bar, "İçe aktarılamadı", "error");
    }
    else document.querySelectorAll(`.${OWN_PREFIX}-toast`).forEach((n) => n.remove());
  } catch (e) {
    toast(document, e instanceof Error ? e.message : "İşlem tamamlanamadı.", "error", 8000);
  } finally {
    running = false;
    setBarBusy(bar, false);
  }
}

function scan(): void {
  for (const item of findMedia(document)) {
    // Tıklamada satır yeniden okunur: görsel bu arada indirilmiş olabilir.
    const host: Element | null = attachBar(item, { onSave: (t) => void onSave(t, () => mediaOf(item.row), host) });
  }
  const viewer = findViewer(document);
  if (viewer && !viewer.toolbar.querySelector(`.${OWN_PREFIX}-bar`)) {
    const item: MediaItem = { row: viewer.toolbar, kind: "image", src: viewer.src, fileName: "", receiptable: true, statement: false, sheet: false };
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

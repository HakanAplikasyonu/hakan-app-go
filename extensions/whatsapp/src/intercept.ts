// Ana dünya yakalayıcısı ve içerik betiği tarafı (main-world.ts'in
// açıklamasına bakın). Testler aynı pencerede ikisini birlikte çalıştırır.

export const ARM = "hakan-wa:arm";
export const FILE = "hakan-wa:file";
export const FAIL = "hakan-wa:fail";

/** Kurulum ne kadar açık kalır (WhatsApp önce dosyayı indirebilir). */
export const ARM_MS = 20_000;

interface Armed {
  token: string;
  until: number;
}

export function installInterceptor(win: Window & typeof globalThis): void {
  const w = win as Window & typeof globalThis & { __hakanWaIntercept?: boolean };
  if (w.__hakanWaIntercept) return;
  w.__hakanWaIntercept = true;
  let armed: Armed | null = null;

  win.addEventListener("message", (ev: MessageEvent) => {
    if (ev.source !== win || ev.data?.type !== ARM || typeof ev.data.token !== "string") return;
    armed = { token: ev.data.token, until: Date.now() + ARM_MS };
  });

  const capture = (a: HTMLAnchorElement): boolean => {
    if (!armed || Date.now() > armed.until) return false;
    const href = a.href;
    if (!href.startsWith("blob:") && !href.startsWith("data:")) return false;
    const { token } = armed;
    armed = null;
    const name = a.download || "belge";
    win
      .fetch(href)
      .then((r) => r.blob())
      .then(
        (blob) => win.postMessage({ type: FILE, token, name, blob }, win.location.origin),
        () => win.postMessage({ type: FAIL, token }, win.location.origin),
      );
    return true;
  };

  const proto = win.HTMLAnchorElement.prototype;
  const click = proto.click;
  proto.click = function (this: HTMLAnchorElement) {
    if (this.hasAttribute("download") && capture(this)) return;
    return click.call(this);
  };
  const dispatch = win.EventTarget.prototype.dispatchEvent;
  win.EventTarget.prototype.dispatchEvent = function (this: EventTarget, ev: Event) {
    if (ev.type === "click" && this instanceof win.HTMLAnchorElement && this.hasAttribute("download") && capture(this)) {
      return false;
    }
    return dispatch.call(this, ev);
  };
}

/** İçerik betiği: yakalayıcıyı kurar, trigger'ı çalıştırır, dosyayı bekler. */
export function captureDownload(win: Window, trigger: () => void, timeoutMs = ARM_MS): Promise<{ name: string; blob: Blob }> {
  const token = Math.random().toString(36).slice(2) + Date.now().toString(36);
  return new Promise((resolve, reject) => {
    const done = (fn: () => void) => {
      win.clearTimeout(timer);
      win.removeEventListener("message", onMessage);
      fn();
    };
    const onMessage = (ev: MessageEvent) => {
      if (ev.source !== win || ev.data?.token !== token) return;
      if (ev.data.type === FILE && ev.data.blob instanceof Blob) {
        done(() => resolve({ name: String(ev.data.name), blob: ev.data.blob }));
      } else if (ev.data.type === FAIL) {
        done(() => reject(new Error("download failed")));
      }
    };
    const timer = win.setTimeout(() => done(() => reject(new Error("download timeout"))), timeoutMs);
    win.addEventListener("message", onMessage);
    win.postMessage({ type: ARM, token }, win.location.origin);
    // Kurulum mesajı sırayla işlenir; tetiklemeyi bir sonraki tura bırak.
    win.setTimeout(trigger, 0);
  });
}

// Sayfaya eklenen arayüz: mesajın altındaki düğme şeridi, mükellef / görev
// seçici ve bildirim. Hepsi kendi gölge DOM'unda (WhatsApp'ın stilleri
// karışmaz); WhatsApp'ın öğelerine yalnızca şeridin kapsayıcısı eklenir.

import { badgeText, badgeTone, type BadgeState } from "./badge";
import type { MediaItem } from "./dom";
import { TARGET_LABELS, type OpenTask, type Target, type Taxpayer } from "./messages";
import { OWN_PREFIX } from "./selectors";

export const BAR_CLASS = `${OWN_PREFIX}-bar`;

const STYLE = `
:host { all: initial; }
.bar { display: flex; gap: 6px; flex-wrap: wrap; margin: 4px 0 2px; font: 500 12px/1.2 system-ui, -apple-system, "Segoe UI", sans-serif; }
button { cursor: pointer; border: 1px solid #c9d3cf; background: #fff; color: #1a2340; border-radius: 999px; padding: 6px 10px; min-height: 28px; font: inherit; }
button:hover:not(:disabled) { background: #eef1ff; }
button:disabled { opacity: .55; cursor: default; }
button.primary { background: #2448FF; border-color: #2448FF; color: #fff; }
.badge { flex-basis: 100%; padding: 4px 10px; border-radius: 999px; width: fit-content; }
.badge[hidden] { display: none; }
.badge.wait { background: #eef1ff; color: #1a2fa8; }
.badge.ok { background: #e3f6ea; color: #116b3a; }
.badge.bad { background: #fde8e8; color: #9b1c1c; }
@media (prefers-color-scheme: dark) {
  button { background: #1f2c33; color: #e9edef; border-color: #3b4a54; }
  .badge.wait { background: #1d2a55; color: #c7d2ff; }
  .badge.ok { background: #12351f; color: #a7e8bf; }
  .badge.bad { background: #451a1a; color: #fcc; }
  button:hover:not(:disabled) { background: #2a3942; }
}
`;

export interface BarHandlers {
  onSave(target: Target, item: MediaItem): void;
}

/** Medya mesajına düğme şeridi ekler (bir kez). */
export function attachBar(item: MediaItem, handlers: BarHandlers): HTMLElement | null {
  if (item.row.querySelector(`.${BAR_CLASS}`)) return null;
  const host = item.row.ownerDocument.createElement("div");
  host.className = BAR_CLASS;
  const root = host.attachShadow({ mode: "open" });
  const style = item.row.ownerDocument.createElement("style");
  style.textContent = STYLE;
  root.append(style);
  const bar = item.row.ownerDocument.createElement("div");
  bar.className = "bar";
  bar.setAttribute("role", "group");
  bar.setAttribute("aria-label", "Easytrade'e kaydet");
  const targets: Target[] = item.receiptable
    ? ["expense", "income", "document", "foreign_invoice"]
    : ["document", "foreign_invoice"];
  for (const t of targets) {
    const b = item.row.ownerDocument.createElement("button");
    b.type = "button";
    b.dataset.target = t;
    b.textContent = TARGET_LABELS[t];
    b.addEventListener("click", (ev) => {
      ev.stopPropagation();
      ev.preventDefault();
      handlers.onSave(t, item);
    });
    bar.append(b);
  }
  const badge = item.row.ownerDocument.createElement("span");
  badge.className = "badge";
  badge.setAttribute("role", "status");
  badge.hidden = true;
  bar.append(badge);
  root.append(bar);
  item.row.append(host);
  return host;
}

/** Şeridin durum rozeti (#780); null gizler. */
export function setBadge(host: Element | null, state: BadgeState | null, taxpayer = ""): void {
  const badge = host?.shadowRoot?.querySelector<HTMLElement>(".badge");
  if (!badge) return;
  if (!state) {
    badge.hidden = true;
    delete badge.dataset.state;
    return;
  }
  badge.hidden = false;
  badge.dataset.state = state;
  badge.className = `badge ${badgeTone(state)}`;
  badge.textContent = badgeText(state, taxpayer);
}

export function badgeOf(host: Element | null): BadgeState | null {
  const s = host?.shadowRoot?.querySelector<HTMLElement>(".badge")?.dataset.state;
  return (s as BadgeState | undefined) ?? null;
}

export function setBarBusy(host: Element | null, busy: boolean): void {
  host?.shadowRoot?.querySelectorAll("button").forEach((b) => (b.disabled = busy));
}

// ---------------------------------------------------------------------------
// Seçici (mükellef, ardından belge görevi)
// ---------------------------------------------------------------------------

const PANEL_STYLE = `
:host { all: initial; }
.backdrop { position: fixed; inset: 0; background: rgba(0,0,0,.35); z-index: 2147483646; display: flex; align-items: center; justify-content: center; }
.panel { width: min(420px, calc(100vw - 32px)); max-height: min(560px, calc(100vh - 32px)); display: flex; flex-direction: column; background: #fff; color: #111b21; border-radius: 12px; box-shadow: 0 12px 40px rgba(0,0,0,.25); font: 14px/1.4 system-ui, -apple-system, "Segoe UI", sans-serif; }
header { padding: 16px 16px 8px; }
h2 { margin: 0 0 4px; font-size: 16px; }
p { margin: 0; color: #54656f; font-size: 13px; }
input[type=search] { margin: 8px 16px; padding: 10px 12px; border: 1px solid #c9d3cf; border-radius: 8px; font: inherit; }
ul { list-style: none; margin: 0; padding: 0 8px; overflow: auto; flex: 1; }
li button { width: 100%; text-align: left; padding: 10px 8px; min-height: 44px; border: 0; background: none; border-radius: 8px; cursor: pointer; font: inherit; color: inherit; }
li button:hover, li button:focus-visible { background: #eef1ff; outline: none; }
.sub { display: block; color: #54656f; font-size: 12px; }
.tag { font-size: 11px; color: #2448FF; font-weight: 600; margin-left: 6px; }
.empty { padding: 12px 8px; color: #54656f; }
footer { display: flex; align-items: center; justify-content: space-between; gap: 8px; padding: 12px 16px 16px; border-top: 1px solid #e9edef; }
label { display: flex; gap: 6px; align-items: center; font-size: 13px; }
.options { display: flex; flex-direction: column; gap: 6px; }
footer button { padding: 8px 14px; min-height: 36px; border-radius: 8px; border: 1px solid #c9d3cf; background: #fff; cursor: pointer; font: inherit; }
@media (prefers-color-scheme: dark) {
  .panel { background: #111b21; color: #e9edef; }
  p, .sub, .empty { color: #8696a0; }
  input[type=search], footer button { background: #202c33; color: #e9edef; border-color: #3b4a54; }
  li button:hover, li button:focus-visible { background: #202c33; }
  footer { border-color: #222d34; }
}
`;

export interface PickTaxpayerOptions {
  chatTitle: string;
  suggestions: Taxpayer[];
  /** Kişi sohbetinin numarası: "telefon olarak kaydet" kutusu yalnızca o zaman çıkar. */
  phone?: string | null;
  search(q: string): Promise<Taxpayer[]>;
}

export interface PickedTaxpayer {
  taxpayer: Taxpayer;
  remember: boolean;
  /** Numara mükellefin telefonu olarak kaydedilsin (bir dahakine otomatik bulunur). */
  savePhone: boolean;
}

function panel(doc: Document): { host: HTMLElement; root: ShadowRoot; close(): void } {
  const host = doc.createElement("div");
  host.className = `${OWN_PREFIX}-picker`;
  const root = host.attachShadow({ mode: "open" });
  const style = doc.createElement("style");
  style.textContent = PANEL_STYLE;
  root.append(style);
  doc.body.append(host);
  return { host, root, close: () => host.remove() };
}

function el<K extends keyof HTMLElementTagNameMap>(doc: Document, tag: K, attrs: Record<string, string> = {}, text = ""): HTMLElementTagNameMap[K] {
  const e = doc.createElement(tag);
  for (const [k, v] of Object.entries(attrs)) e.setAttribute(k, v);
  if (text) e.textContent = text;
  return e;
}

/** İlk kayıtta mükellef seçtirir; iptalde null. */
export function pickTaxpayer(doc: Document, opts: PickTaxpayerOptions): Promise<PickedTaxpayer | null> {
  return new Promise((resolve) => {
    const { root, close } = panel(doc);
    const finish = (v: PickedTaxpayer | null) => {
      close();
      resolve(v);
    };
    const backdrop = el(doc, "div", { class: "backdrop" });
    backdrop.addEventListener("click", (ev) => ev.target === backdrop && finish(null));
    const box = el(doc, "div", { class: "panel", role: "dialog", "aria-modal": "true", "aria-labelledby": "t" });
    const header = el(doc, "header");
    header.append(el(doc, "h2", { id: "t" }, "Mükellef seçin"));
    header.append(el(doc, "p", {}, opts.chatTitle ? `“${opts.chatTitle}” sohbeti hangi mükellefe ait?` : "Bu sohbet hangi mükellefe ait?"));
    const input = el(doc, "input", { type: "search", placeholder: "Unvan ya da VKN / TCKN ile arayın", "aria-label": "Mükellef ara" });
    const list = el(doc, "ul", { role: "listbox", "aria-label": "Mükellefler" });
    const footer = el(doc, "footer");
    const remember = el(doc, "input", { type: "checkbox" });
    remember.checked = true;
    const rememberLabel = el(doc, "label");
    rememberLabel.append(remember, doc.createTextNode("Bu sohbet için hatırla"));
    const savePhone = el(doc, "input", { type: "checkbox" });
    savePhone.checked = true;
    const options = el(doc, "div", { class: "options" });
    options.append(rememberLabel);
    if (opts.phone) {
      const phoneLabel = el(doc, "label");
      phoneLabel.append(savePhone, doc.createTextNode(`Bu numarayı mükellefin telefonu olarak kaydet (${opts.phone})`));
      options.append(phoneLabel);
    }
    const cancel = el(doc, "button", { type: "button" }, "Vazgeç");
    cancel.addEventListener("click", () => finish(null));
    footer.append(options, cancel);

    const render = (items: Taxpayer[], suggested: boolean, emptyText: string) => {
      list.replaceChildren();
      if (items.length === 0) {
        list.append(el(doc, "li", { class: "empty" }, emptyText));
        return;
      }
      for (const t of items) {
        const li = el(doc, "li");
        const b = el(doc, "button", { type: "button", role: "option" });
        b.append(doc.createTextNode(t.title));
        if (suggested) b.append(el(doc, "span", { class: "tag" }, "telefon eşleşti"));
        if (t.tax_id) b.append(el(doc, "span", { class: "sub" }, t.tax_id));
        b.addEventListener("click", () => finish({ taxpayer: t, remember: remember.checked, savePhone: !!opts.phone && savePhone.checked }));
        li.append(b);
        list.append(li);
      }
    };
    render(opts.suggestions, true, "Aramaya başlayın.");

    let seq = 0;
    let timer: ReturnType<typeof setTimeout> | undefined;
    input.addEventListener("input", () => {
      clearTimeout(timer);
      const q = input.value.trim();
      if (q.length < 2) {
        render(opts.suggestions, true, "Aramaya başlayın.");
        return;
      }
      timer = setTimeout(async () => {
        const mine = ++seq;
        list.replaceChildren(el(doc, "li", { class: "empty" }, "Aranıyor…"));
        try {
          const items = await opts.search(q);
          if (mine === seq) render(items, false, "Eşleşen mükellef yok.");
        } catch (e) {
          if (mine === seq) render([], false, e instanceof Error ? e.message : "Arama yapılamadı.");
        }
      }, 250);
    });
    box.addEventListener("keydown", (ev) => ev.key === "Escape" && finish(null));
    box.append(header, input, list, footer);
    backdrop.append(box);
    root.append(backdrop);
    input.focus();
  });
}

/** Açık belge görevleri varken: hangi göreve? undefined = görevsiz, null = iptal. */
export function pickTask(doc: Document, taxpayerTitle: string, tasks: OpenTask[]): Promise<OpenTask | undefined | null> {
  return new Promise((resolve) => {
    const { root, close } = panel(doc);
    const finish = (v: OpenTask | undefined | null) => {
      close();
      resolve(v);
    };
    const backdrop = el(doc, "div", { class: "backdrop" });
    const box = el(doc, "div", { class: "panel", role: "dialog", "aria-modal": "true", "aria-labelledby": "t" });
    const header = el(doc, "header");
    header.append(el(doc, "h2", { id: "t" }, "Bu belge bir talebe mi yanıt?"));
    header.append(el(doc, "p", {}, `${taxpayerTitle} için açık belge talepleri var.`));
    const list = el(doc, "ul", { role: "listbox", "aria-label": "Belge talepleri" });
    for (const t of tasks) {
      const li = el(doc, "li");
      const b = el(doc, "button", { type: "button", role: "option" });
      b.append(doc.createTextNode(t.title || "Belge talebi"));
      if (t.period) b.append(el(doc, "span", { class: "sub" }, t.period));
      b.addEventListener("click", () => finish(t));
      li.append(b);
      list.append(li);
    }
    const footer = el(doc, "footer");
    const none = el(doc, "button", { type: "button" }, "Talepsiz kaydet");
    none.addEventListener("click", () => finish(undefined));
    const cancel = el(doc, "button", { type: "button" }, "Vazgeç");
    cancel.addEventListener("click", () => finish(null));
    footer.append(none, cancel);
    box.addEventListener("keydown", (ev) => ev.key === "Escape" && finish(null));
    box.append(header, list, footer);
    backdrop.append(box);
    root.append(backdrop);
    none.focus();
  });
}

// ---------------------------------------------------------------------------
// Bildirim
// ---------------------------------------------------------------------------

const TOAST_STYLE = `
:host { all: initial; }
div { position: fixed; left: 50%; bottom: 24px; transform: translateX(-50%); z-index: 2147483647; max-width: min(520px, calc(100vw - 32px)); padding: 12px 16px; border-radius: 10px; font: 14px/1.4 system-ui, -apple-system, "Segoe UI", sans-serif; box-shadow: 0 6px 24px rgba(0,0,0,.25); background: #1a2340; color: #fff; }
div.error { background: #8a1c1c; }
`;

export function toast(doc: Document, message: string, kind: "info" | "error" = "info", ms = 5000): void {
  doc.querySelectorAll(`.${OWN_PREFIX}-toast`).forEach((n) => n.remove());
  const host = doc.createElement("div");
  host.className = `${OWN_PREFIX}-toast`;
  const root = host.attachShadow({ mode: "open" });
  const style = doc.createElement("style");
  style.textContent = TOAST_STYLE;
  const box = el(doc, "div", { role: kind === "error" ? "alert" : "status", class: kind }, message);
  root.append(style, box);
  doc.body.append(host);
  setTimeout(() => host.remove(), ms);
}

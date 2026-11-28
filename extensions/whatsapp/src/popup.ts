// Açılır pencere: eşleştirme ve bağlantı durumu.

import type { Reply, Request, Status } from "./messages";
import { defaultBase, normalizeCode, testOutcome } from "./setup";
import { inboxBreakdown, type Summary, type SummaryItem } from "./summary";

declare const __DEFAULT_API_URL__: string;

const $ = <T extends HTMLElement>(id: string) => document.getElementById(id) as T;
const send = <T>(req: Request) => chrome.runtime.sendMessage<Request, Reply<T>>(req);

function showError(msg: string): void {
  const e = $("error");
  e.textContent = msg;
  e.hidden = !msg;
}

function render(s: Status): void {
  $("paired").hidden = !s.paired;
  $("pair").hidden = s.paired;
  if (s.paired) {
    $("org").textContent = s.organization?.name ?? "";
    $("user").textContent = s.user ? `${s.user.name} (${s.user.email})` : "";
    $("server").textContent = s.apiBase;
  } else {
    const base = $<HTMLInputElement>("base");
    if (!base.value) base.value = defaultBase(s.apiBase, __DEFAULT_API_URL__);
    // Adres pakete gömülüyse gizli kalır; yoksa "Gelişmiş" açık gelir.
    if (!base.value) $<HTMLDetailsElement>("advanced").open = true;
    $<HTMLInputElement>("code").focus();
  }
}

/** Bağlantı yeni sekmede (tabs izni gerekmez). */
function linkTo(a: HTMLAnchorElement, url: string): void {
  a.href = url;
  a.onclick = (ev) => {
    ev.preventDefault();
    void chrome.tabs.create({ url }).then(() => window.close());
  };
}

function fillList(id: string, items: SummaryItem[]): void {
  const ul = $(`${id}-list`);
  ul.replaceChildren(
    ...items.map((it) => {
      const li = document.createElement("li");
      const a = document.createElement("a");
      const title = document.createElement("span");
      title.textContent = it.title;
      const detail = document.createElement("small");
      detail.textContent = it.detail;
      a.append(title, detail);
      linkTo(a, it.url);
      li.append(a);
      return li;
    }),
  );
  $(`${id}-empty`).hidden = items.length > 0;
}

function renderSummary(s: (Summary & { webBase: string }) | null): void {
  const web = $<HTMLInputElement>("web");
  if (s && document.activeElement !== web) web.value = s.webBase;
  $("inbox").hidden = !s?.inbox;
  if (s?.inbox) {
    $("inbox-total").textContent = String(s.inbox.counts.total);
    $("inbox-detail").textContent = s.inbox.counts.total ? inboxBreakdown(s.inbox.counts) : "Bekleyen iş yok.";
    linkTo($<HTMLAnchorElement>("inbox"), s.inbox.url);
  }
  $("vat").hidden = !s?.vat;
  if (s?.vat) {
    fillList("vat", s.vat.items);
    linkTo($<HTMLAnchorElement>("vat-all"), s.vat.url);
  }
  $("tasks").hidden = !s?.tasks;
  if (s?.tasks) {
    fillList("tasks", s.tasks.items);
    linkTo($<HTMLAnchorElement>("tasks-all"), s.tasks.url);
  }
}

async function loadSummary(force: boolean): Promise<void> {
  const r = await send<(Summary & { webBase: string }) | null>({ type: "summary", force });
  const err = $("summary-error");
  if (r.ok) {
    renderSummary(r.data);
    err.hidden = true;
  } else {
    err.textContent = "Ofis özeti alınamadı: " + r.error;
    err.hidden = false;
  }
}

async function refresh(): Promise<void> {
  const r = await send<Status>({ type: "status" });
  if (r.ok) {
    render(r.data);
    if (r.data.paired) void loadSummary(false);
  } else {
    showError(r.error);
  }
}

$<HTMLFormElement>("web-form").addEventListener("submit", async (ev) => {
  ev.preventDefault();
  const r = await send<(Summary & { webBase: string }) | null>({ type: "setWebBase", webBase: $<HTMLInputElement>("web").value });
  if (r.ok) renderSummary(r.data);
  else showError(r.error);
});

$<HTMLFormElement>("pair").addEventListener("submit", async (ev) => {
  ev.preventDefault();
  showError("");
  const base = $<HTMLInputElement>("base").value.trim();
  const parsed = normalizeCode($<HTMLInputElement>("code").value);
  const device = $<HTMLInputElement>("device").value.trim() || "Chrome";
  if (!parsed.ok) {
    showError(parsed.error);
    return;
  }
  const code = parsed.code;
  let origin: string;
  try {
    origin = new URL(base).origin;
  } catch {
    $<HTMLDetailsElement>("advanced").open = true;
    showError("Sunucu adresi eksik. Uzantıyı paneldeki WhatsApp sayfasından yeniden indirin ya da Gelişmiş altına adresi yazın.");
    return;
  }
  // Sunucuya istek atabilmek için yalnızca o adrese izin istenir. Pakete
  // gömülü adresin izni manifest'te zaten vardır; pencere çıkmaz.
  const origins = [origin + "/*"];
  const granted = (await chrome.permissions.contains({ origins })) || (await chrome.permissions.request({ origins }));
  if (!granted) {
    showError("Sunucu adresine erişim izni verilmedi.");
    return;
  }
  const submit = $<HTMLButtonElement>("submit");
  submit.disabled = true;
  const r = await send<Status>({ type: "pair", apiBase: base, code, deviceName: device });
  submit.disabled = false;
  if (r.ok) {
    $<HTMLInputElement>("code").value = "";
    render(r.data);
    void loadSummary(true);
  } else {
    showError(r.error);
  }
});

const WHATSAPP_URL = "https://web.whatsapp.com/";

/** Açık WhatsApp Web sekmeleri (web.whatsapp.com için konak izni var; tabs izni gerekmez). */
async function whatsappTabs(): Promise<chrome.tabs.Tab[]> {
  try {
    return await chrome.tabs.query({ url: "https://web.whatsapp.com/*" });
  } catch {
    return [];
  }
}

async function openWhatsApp(): Promise<void> {
  const [tab] = await whatsappTabs();
  if (tab?.id !== undefined) {
    await chrome.tabs.update(tab.id, { active: true });
    if (tab.windowId !== undefined) await chrome.windows.update(tab.windowId, { focused: true });
  } else {
    await chrome.tabs.create({ url: WHATSAPP_URL });
  }
  window.close();
}

$("open-wa").addEventListener("click", () => void openWhatsApp());

$("test").addEventListener("click", async () => {
  const out = $("test-result");
  const btn = $<HTMLButtonElement>("test");
  btn.disabled = true;
  out.hidden = false;
  out.className = "";
  out.textContent = "Deneniyor…";
  const r = await send<boolean>({ type: "test" });
  const tabs = await whatsappTabs();
  const res = testOutcome(r.ok ? "" : r.error, tabs.length);
  btn.disabled = false;
  out.textContent = res.message;
  out.className = res.ok ? "good" : "bad";
  if (!r.ok) void refresh();
});

$("unpair").addEventListener("click", async () => {
  showError("");
  const r = await send<Status>({ type: "unpair" });
  if (r.ok) render(r.data);
  else showError(r.error);
});

void refresh();

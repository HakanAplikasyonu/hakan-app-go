// Açılır pencere: eşleştirme ve bağlantı durumu.

import type { Reply, Request, Status } from "./messages";

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
    if (!base.value) base.value = s.apiBase || __DEFAULT_API_URL__;
  }
}

async function refresh(): Promise<void> {
  const r = await send<Status>({ type: "status" });
  if (r.ok) render(r.data);
  else showError(r.error);
}

$<HTMLFormElement>("pair").addEventListener("submit", async (ev) => {
  ev.preventDefault();
  showError("");
  const base = $<HTMLInputElement>("base").value.trim();
  const code = $<HTMLInputElement>("code").value.trim();
  const device = $<HTMLInputElement>("device").value.trim();
  let origin: string;
  try {
    origin = new URL(base).origin;
  } catch {
    showError("Sunucu adresi geçerli bir adres olmalı (https://…).");
    return;
  }
  // Sunucuya istek atabilmek için yalnızca o adrese izin istenir.
  const granted = await chrome.permissions.request({ origins: [origin + "/*"] });
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
  } else {
    showError(r.error);
  }
});

$("unpair").addEventListener("click", async () => {
  showError("");
  const r = await send<Status>({ type: "unpair" });
  if (r.ok) render(r.data);
  else showError(r.error);
});

void refresh();

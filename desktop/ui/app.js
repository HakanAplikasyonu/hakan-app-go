// Ön yüz yalnızca Rust komutlarını çağırır; sidecar'a doğrudan erişmez.
const { invoke } = window.__TAURI__.core;
const { listen } = window.__TAURI__.event;

const $ = (id) => document.getElementById(id);
const TOKEN_KEY = "api_token";
const URL_KEY = "hakan.apiUrl";

function errText(e) {
  if (e && typeof e === "object" && "code" in e) return `${e.code}: ${e.message}`;
  return String(e);
}

function fill(dl, rows) {
  dl.replaceChildren();
  for (const [k, v] of rows) {
    if (v === undefined || v === null || v === "") continue;
    const dt = document.createElement("dt");
    dt.textContent = k;
    const dd = document.createElement("dd");
    dd.textContent = String(v);
    dl.append(dt, dd);
  }
}

function setBadge(state, text) {
  const b = $("sc-badge");
  b.dataset.state = state;
  b.textContent = text;
}

function showReady(info) {
  setBadge("ok", "Çalışıyor");
  fill($("sc-info"), [
    ["Sürüm", `${info.version} (${info.commit})`],
    ["Platform", `${info.os}/${info.arch}`],
    ["Go", info.go_version],
    ["PID", info.pid],
  ]);
}

async function refreshStatus() {
  const s = await invoke("sidecar_status");
  if (s.ready) showReady(s.ready);
  else if (!s.alive) setBadge("down", "Kapalı");
}

listen("sidecar://event", (e) => {
  if (e.payload.event === "ready") showReady(e.payload.data);
});
listen("sidecar://exit", () => setBadge("down", "Kapandı"));

$("sc-ping").addEventListener("click", async () => {
  const t0 = performance.now();
  try {
    await invoke("sidecar_call", { method: "ping", params: { echo: "merhaba" } });
    $("sc-ping-out").textContent = `Yanıt ${(performance.now() - t0).toFixed(1)} ms`;
  } catch (e) {
    $("sc-ping-out").textContent = errText(e);
  }
});

$("sc-restart").addEventListener("click", async () => {
  setBadge("wait", "Başlatılıyor…");
  try {
    await invoke("sidecar_restart");
  } catch (e) {
    setBadge("down", errText(e));
  }
});

$("api-url").value = localStorage.getItem(URL_KEY) ?? "";
$("api-form").addEventListener("submit", async (ev) => {
  ev.preventDefault();
  const base_url = $("api-url").value.trim();
  localStorage.setItem(URL_KEY, base_url);
  $("api-out").textContent = "Deneniyor…";
  try {
    const r = await invoke("sidecar_call", { method: "api.health", params: { base_url } });
    $("api-out").textContent = `Bağlandı: ${r.app_name || "API"} · ${r.latency_ms} ms`;
  } catch (e) {
    $("api-out").textContent = errText(e);
  }
});

async function tokenState() {
  try {
    const has = await invoke("secret_exists", { key: TOKEN_KEY });
    $("token-out").textContent = has ? "Anahtar zincirinde kayıtlı." : "Kayıtlı anahtar yok.";
  } catch (e) {
    $("token-out").textContent = `Anahtar zincirine erişilemedi: ${errText(e)}`;
  }
}

$("token-form").addEventListener("submit", async (ev) => {
  ev.preventDefault();
  const value = $("token").value;
  if (!value) return;
  try {
    await invoke("secret_set", { key: TOKEN_KEY, value });
    $("token").value = "";
  } catch (e) {
    $("token-out").textContent = errText(e);
    return;
  }
  tokenState();
});

$("token-delete").addEventListener("click", async () => {
  await invoke("secret_delete", { key: TOKEN_KEY }).catch(() => {});
  tokenState();
});

listen("tauri://drag-drop", async (e) => {
  const path = (e.payload.paths || []).find((p) => p.toLowerCase().endsWith(".xml"));
  if (!path) {
    fill($("ubl-out"), [["Hata", "Yalnızca .xml dosyası"]]);
    return;
  }
  try {
    const s = await invoke("sidecar_call", { method: "ubl.inspect", params: { path } });
    fill($("ubl-out"), [
      ["Dosya", path.split(/[\\/]/).pop()],
      ["Tür", s.kind],
      ["Numara", s.id],
      ["ETTN", s.uuid],
      ["Senaryo / tip", `${s.profile} / ${s.type}`],
      ["Satıcı", s.supplier],
      ["Alıcı", s.customer],
      ["Satır", s.lines],
      ["Vergi", s.tax],
      ["Ödenecek", `${s.payable} ${s.currency}`],
    ]);
  } catch (e) {
    fill($("ubl-out"), [["Hata", errText(e)]]);
  }
});

$("up-check").addEventListener("click", async () => {
  $("up-out").textContent = "Denetleniyor…";
  try {
    const u = await invoke("update_check");
    if (!u.enabled) $("up-out").textContent = `Sürüm ${u.current}. Bu derlemede güncelleyici kapalı.`;
    else if (u.available) $("up-out").textContent = `Yeni sürüm: ${u.available}`;
    else $("up-out").textContent = `Sürüm ${u.current} güncel.`;
  } catch (e) {
    $("up-out").textContent = errText(e);
  }
});

window.__TAURI__.app
  .getVersion()
  .then((v) => ($("app-version").textContent = `v${v}`))
  .catch(() => {});
refreshStatus();
tokenState();

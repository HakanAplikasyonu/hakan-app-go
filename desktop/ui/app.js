// Ön yüz yalnızca Rust komutlarını çağırır; sidecar'a doğrudan erişmez.
// Oturum belirteci ön yüze hiç gelmez (kabuk anahtar zincirinde tutar).
const { invoke } = window.__TAURI__.core;
const { listen } = window.__TAURI__.event;

const $ = (id) => document.getElementById(id);
const call = (method, params = {}) => invoke("sidecar_call", { method, params });
const UPLOAD_LOG = 20;

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

function badge(el, state, text) {
  el.dataset.state = state;
  el.textContent = text;
}

const fmtTime = (iso) => (iso ? new Date(iso).toLocaleString("tr-TR") : "");

// --- Ajan durumu ----------------------------------------------------------

async function refresh() {
  let st;
  try {
    st = await call("agent.status");
  } catch (e) {
    $("login-out").textContent = errText(e);
    return;
  }
  if (st.base_url && !$("api-url").value) $("api-url").value = st.base_url;
  const running = st.signed_in && st.running;
  $("login-form").hidden = st.signed_in;
  $("signed-in").hidden = !running;
  $("org-pick").hidden = !st.signed_in || running;
  if (!st.signed_in) badge($("agent-badge"), "wait", "Oturum yok");
  else if (!running) badge($("agent-badge"), "wait", "Ofis seçilmedi");
  else if (st.heartbeat_error) badge($("agent-badge"), "down", "Bağlantı sorunu");
  else badge($("agent-badge"), "ok", "Çevrimiçi");

  fill($("agent-info"), [
    ["Ofis", st.organization?.name],
    ["Ajan", st.agent_name],
    ["Son heartbeat", fmtTime(st.last_heartbeat)],
    ["Hata", st.heartbeat_error],
    ["Yüklenen dosya", st.uploaded_total],
  ]);
  renderWatches(st.watches);
  if (running && !$("engagement").options.length) loadEngagements();
}

function renderWatches(watches) {
  const ul = $("watches");
  ul.replaceChildren();
  for (const w of watches) {
    const li = document.createElement("li");
    const span = document.createElement("span");
    span.textContent = `${w.label || w.path} → ${w.engagement_uuid.slice(0, 8)}`;
    span.title = w.path;
    const btn = document.createElement("button");
    btn.className = "ghost small-btn";
    btn.textContent = "Kaldır";
    btn.onclick = async () => {
      await call("watch.remove", { path: w.path }).catch(() => {});
      refresh();
    };
    li.append(span, btn);
    ul.append(li);
  }
  if (!watches.length) {
    const li = document.createElement("li");
    li.className = "muted";
    li.textContent = "İzlenen klasör yok.";
    ul.append(li);
  }
}

async function loadEngagements() {
  try {
    const { items } = await call("agent.engagements");
    const sel = $("engagement");
    sel.replaceChildren();
    for (const e of items) {
      const o = document.createElement("option");
      o.value = e.uuid;
      o.textContent = `Mükellef ${e.taxpayer_uuid.slice(0, 8)} · ilişki ${e.uuid.slice(0, 8)}`;
      sel.append(o);
    }
  } catch (e) {
    $("login-out").textContent = errText(e);
  }
}

// --- Oturum -----------------------------------------------------------------

$("api-check").addEventListener("click", async () => {
  $("login-out").textContent = "Deneniyor…";
  try {
    const r = await call("api.health", { base_url: $("api-url").value.trim() });
    $("login-out").textContent = `Bağlandı: ${r.app_name || "API"} · ${r.latency_ms} ms`;
  } catch (e) {
    $("login-out").textContent = errText(e);
  }
});

$("login-form").addEventListener("submit", async (ev) => {
  ev.preventDefault();
  $("login-out").textContent = "Oturum açılıyor…";
  try {
    const { organizations } = await call("auth.login", {
      base_url: $("api-url").value.trim(),
      email: $("email").value.trim(),
      password: $("password").value,
      totp_code: $("totp").value.trim(),
    });
    $("password").value = "";
    $("totp").value = "";
    const sel = $("org");
    sel.replaceChildren();
    for (const o of organizations.filter((x) => x.status === "active")) {
      const opt = document.createElement("option");
      opt.value = o.slug;
      opt.textContent = o.name;
      sel.append(opt);
    }
    $("login-out").textContent = organizations.length ? "" : "Bu hesabın ofis üyeliği yok.";
  } catch (e) {
    if (e?.code === "MFA_REQUIRED") {
      $("totp-row").hidden = false;
      $("login-out").textContent = "Doğrulama kodunu girin.";
    } else {
      $("login-out").textContent = errText(e);
    }
  }
  refresh();
});

$("org-use").addEventListener("click", async () => {
  try {
    await call("agent.use_organization", { slug: $("org").value, agent_name: $("agent-name").value.trim() });
    $("login-out").textContent = "";
  } catch (e) {
    $("login-out").textContent = errText(e);
  }
  refresh();
});

$("logout").addEventListener("click", async () => {
  await call("auth.logout").catch(() => {});
  $("engagement").replaceChildren();
  refresh();
});

// --- Klasör izleme ----------------------------------------------------------

async function addWatch(path) {
  try {
    await call("watch.add", {
      path,
      engagement_uuid: $("engagement").value,
      label: $("watch-label").value.trim(),
    });
    $("watch-path").value = "";
    $("watch-label").value = "";
  } catch (e) {
    $("login-out").textContent = errText(e);
  }
  refresh();
}

$("watch-form").addEventListener("submit", (ev) => {
  ev.preventDefault();
  addWatch($("watch-path").value.trim());
});

const outcomeText = {
  uploaded: "yüklendi",
  rejected: "reddedildi",
  failed: "gönderilemedi, yeniden denenecek",
};

listen("sidecar://event", (e) => {
  const { event, data } = e.payload;
  if (event === "ready") showReady(data);
  if (event === "upload") {
    const li = document.createElement("li");
    li.dataset.outcome = data.outcome;
    li.textContent = `${new Date().toLocaleTimeString("tr-TR")} · ${data.name} · ${outcomeText[data.outcome] ?? data.outcome}${data.error ? ` (${data.error})` : ""}`;
    const ul = $("uploads");
    ul.prepend(li);
    while (ul.children.length > UPLOAD_LOG) ul.lastChild.remove();
  }
  if (["agent.heartbeat", "auth.signed_out", "agent.revoked", "upload"].includes(event)) refresh();
});

// --- Logo / Mikro -------------------------------------------------------------

function srcConfig() {
  const kind = $("src-kind").value;
  const cfg = {
    kind,
    host: $("src-host").value.trim(),
    database: $("src-db").value.trim(),
    user: $("src-user").value.trim(),
    password: $("src-pass").value,
  };
  const port = Number($("src-port").value);
  if (port) cfg.port = port;
  if (kind !== "mikro") {
    cfg.firm_no = $("src-firm").value.trim();
    cfg.period_no = $("src-period").value.trim();
  }
  return cfg;
}

$("src-kind").addEventListener("change", () => {
  $("logo-only").hidden = $("src-kind").value === "mikro";
});

$("src-form").addEventListener("submit", async (ev) => {
  ev.preventDefault();
  $("src-out").textContent = "Bağlanılıyor…";
  try {
    const r = await call("source.test", { config: srcConfig() });
    $("src-out").textContent = JSON.stringify(r, null, 2);
  } catch (e) {
    $("src-out").textContent = errText(e);
  }
});

$("src-preview").addEventListener("click", async () => {
  const now = new Date();
  const from = new Date(Date.UTC(now.getUTCFullYear(), now.getUTCMonth() - 1, 1));
  const to = new Date(Date.UTC(now.getUTCFullYear(), now.getUTCMonth(), 1));
  const day = (d) => d.toISOString().slice(0, 10);
  $("src-out").textContent = "Okunuyor…";
  try {
    const r = await call("source.preview", { config: srcConfig(), from: day(from), to: day(to) });
    $("src-out").textContent = JSON.stringify({ ...r, sample: r.sample?.slice(0, 5) }, null, 2);
  } catch (e) {
    $("src-out").textContent = errText(e);
  }
});

// --- Sidecar, UBL, güncelleme ---------------------------------------------

function showReady(info) {
  badge($("sc-badge"), "ok", "Çalışıyor");
  fill($("sc-info"), [
    ["Sürüm", `${info.version} (${info.commit})`],
    ["Platform", `${info.os}/${info.arch}`],
    ["PID", info.pid],
  ]);
}

listen("sidecar://exit", () => badge($("sc-badge"), "down", "Kapandı"));

$("sc-ping").addEventListener("click", async () => {
  const t0 = performance.now();
  try {
    await call("ping", { echo: "merhaba" });
    $("sc-out").textContent = `Yanıt ${(performance.now() - t0).toFixed(1)} ms`;
  } catch (e) {
    $("sc-out").textContent = errText(e);
  }
});

$("sc-restart").addEventListener("click", async () => {
  badge($("sc-badge"), "wait", "Başlatılıyor…");
  try {
    await invoke("sidecar_restart");
  } catch (e) {
    badge($("sc-badge"), "down", errText(e));
  }
});

// --- Güncellemeler (#205) ---------------------------------------------------
// Kabuk açılışta ve 6 saatte bir denetler; durum `updater://status`,
// indirme ilerlemesi `updater://progress` olayıyla gelir.

const fmtBytes = (n) => (n >= 1048576 ? `${(n / 1048576).toFixed(1)} MB` : `${Math.max(1, Math.round(n / 1024))} KB`);

function updateText(u) {
  if (!u.enabled) return u.disabled_reason || "Bu derlemede otomatik güncelleme kapalı.";
  const checked = u.checked_at ? ` Son denetim: ${fmtTime(u.checked_at)}.` : "";
  switch (u.state) {
    case "checking":
      return "Güncelleme denetleniyor…";
    case "available":
      return `Yeni sürüm hazır: ${u.available} (kurulu: ${u.current}).${u.notes ? ` ${u.notes}` : ""}`;
    case "downloading":
      return `Sürüm ${u.available} indiriliyor…`;
    case "installing":
      return "Güncelleme kuruluyor, uygulama yeniden başlatılacak…";
    case "up_to_date":
      return `Sürüm ${u.current} güncel.${checked}`;
    case "error":
      return `${u.error || "Güncelleme denetlenemedi."}${checked}`;
    default:
      return `Sürüm ${u.current}. Güncellemeler otomatik denetlenir.`;
  }
}

const updateBadge = {
  disabled: ["wait", "Kapalı"],
  idle: ["wait", "Denetlenmedi"],
  checking: ["wait", "Denetleniyor…"],
  up_to_date: ["ok", "Güncel"],
  available: ["wait", "Yeni sürüm var"],
  downloading: ["wait", "İndiriliyor…"],
  installing: ["wait", "Kuruluyor…"],
  error: ["down", "Sorun var"],
};

function showUpdate(u) {
  const [state, text] = updateBadge[u.state] || updateBadge.idle;
  badge($("up-badge"), state, text);
  $("up-text").textContent = updateText(u);
  $("up-channel").value = u.channel;
  $("up-channel").disabled = !u.enabled;
  $("up-check").disabled = !u.enabled || ["checking", "downloading", "installing"].includes(u.state);
  const busy = u.state === "downloading" || u.state === "installing";
  $("up-install").hidden = u.state !== "available" && !busy;
  $("up-install").disabled = busy;
  $("update-banner-install").disabled = busy;
  $("up-progress-row").hidden = !busy;
  const banner = u.state === "available" || busy;
  $("update-banner").hidden = !banner;
  if (banner) $("update-banner-text").textContent = `Hakan ${u.available} sürümü hazır.`;
}

function showProgress({ downloaded, total }) {
  const bar = $("up-progress");
  if (total) {
    bar.max = total;
    bar.value = downloaded;
    $("up-progress-text").textContent = `${fmtBytes(downloaded)} / ${fmtBytes(total)} (%${Math.floor((downloaded / total) * 100)})`;
  } else {
    bar.removeAttribute("value");
    $("up-progress-text").textContent = `${fmtBytes(downloaded)} indirildi`;
  }
}

async function installUpdate() {
  $("up-progress-row").hidden = false;
  try {
    await invoke("update_install"); // başarıda uygulama yeniden başlar
  } catch (e) {
    $("up-text").textContent = typeof e === "string" ? e : "Güncelleme kurulamadı.";
  }
}

$("up-check").addEventListener("click", () => invoke("update_check").then(showUpdate).catch(() => {}));
$("up-channel").addEventListener("change", () =>
  invoke("update_set_channel", { channel: $("up-channel").value })
    .then(showUpdate)
    .catch((e) => ($("up-text").textContent = typeof e === "string" ? e : "Kanal değiştirilemedi.")),
);
$("up-install").addEventListener("click", installUpdate);
$("update-banner-install").addEventListener("click", installUpdate);
listen("updater://status", (e) => showUpdate(e.payload));
listen("updater://progress", (e) => showProgress(e.payload));
invoke("update_status").then(showUpdate).catch(() => {});

// Sürükle-bırak: .xml → yerel denetim; klasör → izlemeye ekle.
listen("tauri://drag-drop", async (e) => {
  for (const path of e.payload.paths || []) {
    if (path.toLowerCase().endsWith(".xml")) {
      try {
        const s = await call("ubl.inspect", { path });
        fill($("ubl-out"), [
          ["Dosya", path.split(/[\\/]/).pop()],
          ["Numara", s.id],
          ["ETTN", s.uuid],
          ["Senaryo / tip", `${s.profile} / ${s.type}`],
          ["Satıcı", s.supplier],
          ["Alıcı", s.customer],
          ["Ödenecek", `${s.payable} ${s.currency}`],
        ]);
      } catch (err) {
        fill($("ubl-out"), [["Hata", errText(err)]]);
      }
    } else if (!/\.[a-z0-9]{2,4}$/i.test(path)) {
      if ($("engagement").value) addWatch(path);
      else $("watch-path").value = path;
    }
  }
});

window.__TAURI__.app
  .getVersion()
  .then((v) => ($("app-version").textContent = `v${v}`))
  .catch(() => {});
invoke("sidecar_status").then((s) => s.ready && showReady(s.ready));
refresh();
setInterval(refresh, 30000);

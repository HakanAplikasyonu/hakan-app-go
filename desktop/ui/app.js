// Hakan masaüstü arayüzü (#206). Ön yüz yalnızca Rust komutlarını çağırır;
// sidecar'a doğrudan erişmez, oturum belirteci ön yüze hiç gelmez.
//
// Kural: hiçbir ekranda UUID, iç kod ya da ham JSON gösterilmez. Mükellefler
// adlarıyla, hatalar Türkçe açıklamalarıyla görünür. Tanılama araçları
// "Gelişmiş" sekmesindedir.
const { invoke } = window.__TAURI__.core;
const { listen } = window.__TAURI__.event;

const $ = (id) => document.getElementById(id);
const call = (method, params = {}) => invoke("sidecar_call", { method, params });
const FEED_SIZE = 30;

// --- Biçimlendirme ----------------------------------------------------------

const UUID_RE = /[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}/i;

const ERROR_TEXT = {
  INVALID_CREDENTIALS: "E-posta ya da parola hatalı.",
  INVALID_MFA_CODE: "Doğrulama kodu hatalı ya da süresi geçmiş.",
  MFA_NOT_ENROLLED: "Hesabınızda iki adımlı doğrulama kurulu değil. Web panelinden kurun.",
  ACCOUNT_DEACTIVATED: "Hesabınız devre dışı. Ofis yöneticinize başvurun.",
  RATE_LIMITED: "Çok fazla deneme yapıldı. Birkaç dakika sonra yeniden deneyin.",
  VALIDATION_ERROR: "Girilen bilgiler geçersiz.",
  FORBIDDEN: "Bu işlem için yetkiniz yok.",
  TENANT_FORBIDDEN: "Bu ofiste bu işlem için yetkiniz yok.",
  SIGNED_OUT: "Oturumunuz kapandı. Yeniden oturum açın.",
  UNAUTHENTICATED: "Oturumunuz kapandı. Yeniden oturum açın.",
  SIDECAR_DOWN: "Yerel ajan çalışmıyor; birkaç saniye içinde yeniden başlatılacak.",
  UNAVAILABLE: "Sunucuya ulaşılamadı. İnternet bağlantınızı ve sunucu adresini denetleyin.",
  TIMEOUT: "Sunucu zamanında yanıt vermedi. Yeniden deneyin.",
  NOT_FOUND: "Kayıt bulunamadı.",
};

const GENERIC_ERROR = "Beklenmeyen bir sorun oluştu. Sorun sürerse Gelişmiş bölümündeki bilgileri destek ekibine iletin.";
let lastTechnicalError = "";

/** Hatayı kullanıcıya gösterilecek Türkçe metne çevirir. */
export function errText(e) {
  const code = e && typeof e === "object" ? e.code : undefined;
  const msg = e && typeof e === "object" ? String(e.message ?? "") : String(e ?? "");
  lastTechnicalError = [code, msg].filter(Boolean).join(" · ");
  if (code && ERROR_TEXT[code]) return ERROR_TEXT[code];
  // Sidecar'ın doğrulama iletileri Türkçe ve kısadır (ör. "klasör bulunamadı").
  if (msg && msg.length < 160 && !UUID_RE.test(msg) && !/[{}[\]]/.test(msg) && /[çğıöşüÇĞİÖŞÜ]|\b(ve|bir|için|olmalı|gerekli|seçilmeli|bulunamadı)\b/.test(msg)) {
    return msg.charAt(0).toUpperCase() + msg.slice(1) + (/[.!?]$/.test(msg) ? "" : ".");
  }
  return GENERIC_ERROR;
}

const fmtTime = (iso) => {
  if (!iso) return "";
  const d = new Date(iso);
  if (Number.isNaN(d.getTime()) || d.getFullYear() < 2000) return "";
  return d.toLocaleString("tr-TR", { dateStyle: "short", timeStyle: "short" });
};

function relTime(iso) {
  if (!iso) return "";
  const s = (Date.now() - new Date(iso).getTime()) / 1000;
  if (s < 60) return "az önce";
  if (s < 3600) return `${Math.floor(s / 60)} dk önce`;
  if (s < 86400) return `${Math.floor(s / 3600)} sa önce`;
  return fmtTime(iso);
}

const folderName = (p) => (p || "").split(/[\\/]/).filter(Boolean).pop() || p;

function show(el, text) {
  el.textContent = text || "";
  el.hidden = !text;
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

function el(tag, props = {}, ...children) {
  const n = document.createElement(tag);
  for (const [k, v] of Object.entries(props)) {
    if (k === "class") n.className = v;
    else if (k === "dataset") Object.assign(n.dataset, v);
    else if (k.startsWith("on")) n.addEventListener(k.slice(2), v);
    else if (v !== undefined && v !== null && v !== false) n.setAttribute(k, v === true ? "" : v);
  }
  for (const c of children) if (c !== null && c !== undefined && c !== false) n.append(c);
  return n;
}

let confirmResolve = null;
function confirmBox(text, yes = "Kaldır") {
  $("confirm-text").textContent = text;
  $("confirm-yes").textContent = yes;
  $("confirm").showModal();
  return new Promise((r) => (confirmResolve = r));
}
$("confirm-yes").addEventListener("click", () => {
  $("confirm").close();
  confirmResolve?.(true);
});
$("confirm-no").addEventListener("click", () => {
  $("confirm").close();
  confirmResolve?.(false);
});

// --- Görünümler -------------------------------------------------------------

const VIEWS = ["loading", "login", "office", "folders", "program", "settings", "advanced"];
let currentTab = "folders";
let currentView = "loading";

function setView(name) {
  currentView = name;
  for (const v of VIEWS) $(`view-${v}`).hidden = v !== name;
  const inApp = ["folders", "program", "settings", "advanced"].includes(name);
  $("tabs").hidden = !inApp;
  for (const b of $("tabs").querySelectorAll("[role=tab]")) {
    b.setAttribute("aria-selected", String(b.dataset.view === name));
  }
}

$("tabs").addEventListener("click", (e) => {
  const v = e.target.closest("[role=tab]")?.dataset.view;
  if (!v) return;
  currentTab = v;
  setView(v);
  if (v === "settings") loadSettings();
});

// --- Ajan durumu ------------------------------------------------------------

let status = null;
const engagementTitles = new Map(); // engagement → mükellef adı
let engagements = [];

async function refresh() {
  let st;
  try {
    st = await call("agent.status");
  } catch (e) {
    if (currentView === "loading" || e?.code === "SIDECAR_DOWN") {
      badge($("agent-badge"), "wait", "Yerel ajan başlatılıyor…");
    }
    return;
  }
  status = st;
  if (st.base_url && !$("api-url").value) $("api-url").value = st.base_url;
  if (!st.base_url) $("server-box").open = true;

  const running = st.signed_in && st.running;
  $("office-name").textContent = running ? st.organization?.name || "" : "";
  if (!st.signed_in) {
    badge($("agent-badge"), "wait", "Oturum açılmadı");
    if (currentView !== "login") setView("login");
    return;
  }
  if (!running) {
    badge($("agent-badge"), "wait", "Ofis seçilmedi");
    if (currentView !== "office") {
      setView("office");
      loadOffices();
    }
    return;
  }
  if (paused) badge($("agent-badge"), "wait", "Duraklatıldı");
  else if (st.heartbeat_error) badge($("agent-badge"), "down", "Bağlantı sorunu");
  else badge($("agent-badge"), "ok", "Çevrimiçi");
  if (!["folders", "program", "settings", "advanced"].includes(currentView)) setView(currentTab);

  $("sum-conn").textContent = st.heartbeat_error ? "Sunucuya ulaşılamıyor" : "Bağlı";
  $("sum-beat").textContent = relTime(st.last_heartbeat) || "—";
  $("sum-folders").textContent = String(st.watches.length);
  $("sum-uploaded").textContent = String(st.uploaded_total ?? 0);
  fill($("account-info"), [
    ["Ofis", st.organization?.name],
    ["Bu bilgisayar", st.agent_name],
    ["Sunucu", st.base_url],
  ]);
  if (!engagements.length) await loadEngagements();
  renderWatches(st.watches);
}

// --- Oturum -----------------------------------------------------------------

$("api-check").addEventListener("click", async () => {
  show($("login-msg"), "Deneniyor…");
  $("login-msg").dataset.kind = "info";
  try {
    await call("api.health", { base_url: $("api-url").value.trim() });
    show($("login-msg"), "Sunucuya bağlanıldı.");
    $("login-msg").dataset.kind = "ok";
  } catch (e) {
    show($("login-msg"), errText(e));
    $("login-msg").dataset.kind = "error";
  }
});

let organizations = [];

$("login-form").addEventListener("submit", async (ev) => {
  ev.preventDefault();
  const email = $("email").value.trim();
  const password = $("password").value;
  $("login-msg").dataset.kind = "error";
  if (!email || !password) return show($("login-msg"), "E-posta ve parolanızı girin.");
  if (!$("api-url").value.trim()) {
    $("server-box").open = true;
    return show($("login-msg"), "Sunucu adresini girin.");
  }
  $("login-submit").disabled = true;
  show($("login-msg"), "");
  try {
    const r = await call("auth.login", {
      base_url: $("api-url").value.trim(),
      email,
      password,
      totp_code: $("totp").value.trim(),
    });
    organizations = (r.organizations || []).filter((o) => o.status === "active");
    $("password").value = "";
    $("totp").value = "";
    $("totp-row").hidden = true;
    renderOffices();
  } catch (e) {
    if (e?.code === "MFA_REQUIRED") {
      $("totp-row").hidden = false;
      $("totp").focus();
      $("login-msg").dataset.kind = "info";
      show($("login-msg"), "Hesabınızda iki adımlı doğrulama açık. Doğrulama kodunu girin.");
    } else {
      show($("login-msg"), errText(e));
    }
  } finally {
    $("login-submit").disabled = false;
  }
  refresh();
});

async function loadOffices() {
  if (organizations.length) return renderOffices();
  try {
    const r = await call("agent.organizations");
    organizations = (r.organizations || []).filter((o) => o.status === "active");
  } catch (e) {
    show($("office-msg"), errText(e));
  }
  renderOffices();
}

function renderOffices() {
  const box = $("office-list");
  box.replaceChildren();
  organizations.forEach((o, i) => {
    box.append(
      el(
        "label",
        { class: "choice" },
        el("input", { type: "radio", name: "office", value: o.slug, checked: i === 0 }),
        el("span", {}, o.name),
      ),
    );
  });
  show($("office-msg"), organizations.length ? "" : "Bu hesabın etkin bir ofis üyeliği yok.");
  $("office-use").disabled = !organizations.length;
}

$("office-use").addEventListener("click", async () => {
  const slug = document.querySelector("input[name=office]:checked")?.value;
  if (!slug) return show($("office-msg"), "Bir ofis seçin.");
  $("office-use").disabled = true;
  try {
    await call("agent.use_organization", { slug, agent_name: $("agent-name").value.trim() });
    show($("office-msg"), "");
    engagements = [];
  } catch (e) {
    show($("office-msg"), errText(e));
  } finally {
    $("office-use").disabled = false;
  }
  refresh();
});

async function signOut() {
  await call("auth.logout").catch(() => {});
  organizations = [];
  engagements = [];
  engagementTitles.clear();
  $("uploads").replaceChildren();
  setView("login");
  refresh();
}
$("office-logout").addEventListener("click", signOut);
$("logout").addEventListener("click", async () => {
  if (await confirmBox("Oturumu kapatırsanız klasör izleme durur. Devam edilsin mi?", "Oturumu kapat")) signOut();
});

// --- Mükellefler ------------------------------------------------------------

async function loadEngagements() {
  try {
    const { items } = await call("agent.engagements");
    engagements = (items || [])
      .map((e) => ({ id: e.uuid, title: (e.taxpayer_title || "").trim() || "Adı girilmemiş mükellef" }))
      .sort((a, b) => a.title.localeCompare(b.title, "tr"));
    for (const e of engagements) engagementTitles.set(e.id, e.title);
  } catch (e) {
    show($("add-msg"), errText(e));
  }
  renderEngagements();
}

function renderEngagements() {
  const q = $("taxpayer-search").value.trim().toLocaleLowerCase("tr");
  const sel = $("engagement");
  const keep = sel.value;
  sel.replaceChildren();
  for (const e of engagements) {
    if (q && !e.title.toLocaleLowerCase("tr").includes(q)) continue;
    sel.append(el("option", { value: e.id }, e.title));
  }
  if (keep) sel.value = keep;
  if (!sel.value && sel.options.length) sel.selectedIndex = 0;
  $("engagement-empty").hidden = engagements.length > 0;
}
$("taxpayer-search").addEventListener("input", renderEngagements);

const taxpayerOf = (w) => w.taxpayer_title || engagementTitles.get(w.engagement_uuid) || "Mükellef";

// --- Klasörler --------------------------------------------------------------

let pickedFolder = "";

function setPicked(path) {
  pickedFolder = path || "";
  $("picked-folder").textContent = pickedFolder || "Henüz seçilmedi. Klasörü pencereye sürükleyebilirsiniz.";
  $("picked-folder").className = pickedFolder ? "path" : "muted";
}

function openAdd() {
  $("add-form").hidden = false;
  $("add-open").hidden = true;
  show($("add-msg"), "");
  if (!engagements.length) loadEngagements();
  $("taxpayer-search").focus();
}
function closeAdd() {
  $("add-form").hidden = true;
  $("add-open").hidden = false;
  setPicked("");
  $("watch-label").value = "";
  $("taxpayer-search").value = "";
  renderEngagements();
}
$("add-open").addEventListener("click", openAdd);
$("add-cancel").addEventListener("click", closeAdd);

$("pick-folder").addEventListener("click", async () => {
  try {
    const p = await invoke("pick_folder");
    if (p) setPicked(p);
  } catch (e) {
    show($("add-msg"), errText(e));
  }
});

$("add-form").addEventListener("submit", async (ev) => {
  ev.preventDefault();
  const id = $("engagement").value;
  if (!id) return show($("add-msg"), "Bir mükellef seçin.");
  if (!pickedFolder) return show($("add-msg"), "İzlenecek klasörü seçin.");
  try {
    await call("watch.add", {
      path: pickedFolder,
      engagement_uuid: id,
      label: $("watch-label").value.trim(),
      taxpayer_title: engagementTitles.get(id) || "",
    });
    closeAdd();
  } catch (e) {
    show($("add-msg"), errText(e));
  }
  refresh();
});

function watchState(a) {
  if (a.folder_error) return ["down", a.folder_error];
  if (a.errors?.length) return ["warn", `${a.errors.length} dosyada sorun var`];
  if (a.last_upload_at) return ["ok", `Son yükleme ${relTime(a.last_upload_at)}`];
  return ["wait", "Yeni dosya bekleniyor"];
}

// Açık bırakılan hata listeleri yenilemede kapanmasın.
const openErrorBoxes = new Set();

function renderWatches(watches) {
  const ul = $("watches");
  ul.replaceChildren();
  if (!watches.length) {
    ul.append(el("li", { class: "empty" }, "Henüz izlenen klasör yok. “Klasör ekle” ile başlayın."));
    return;
  }
  for (const w of watches) {
    const a = w.activity || { errors: [] };
    const [state, text] = watchState(a);
    const remove = el("button", { type: "button", class: "ghost small-btn" }, "Kaldır");
    remove.addEventListener("click", async () => {
      if (!(await confirmBox(`“${w.label || folderName(w.path)}” klasörü izlemeden çıkarılsın mı? Klasördeki dosyalara dokunulmaz.`))) return;
      await call("watch.remove", { path: w.path }).catch(() => {});
      refresh();
    });
    const li = el(
      "li",
      { class: "watch", dataset: { state } },
      el(
        "div",
        { class: "watch-head" },
        el(
          "div",
          { class: "watch-title" },
          el("strong", {}, w.label || folderName(w.path)),
          el("span", { class: "muted" }, taxpayerOf(w)),
        ),
        el("span", { class: "status", dataset: { state } }, text),
        remove,
      ),
      el("div", { class: "path", title: w.path }, w.path),
      el(
        "div",
        { class: "muted small" },
        `${a.uploaded || 0} dosya yüklendi (bu oturumda)` + (a.last_file ? ` · Son dosya: ${a.last_file}` : ""),
      ),
    );
    if (a.errors?.length) {
      const list = el("ul", { class: "errors" });
      for (const e of a.errors) {
        list.append(
          el(
            "li",
            {},
            el("span", { class: "file" }, e.name),
            el("span", {}, e.message),
            el("span", { class: "muted" }, fmtTime(e.at)),
          ),
        );
      }
      const clear = el("button", { type: "button", class: "ghost small-btn" }, "Listeyi temizle");
      clear.addEventListener("click", async () => {
        await call("watch.clear_errors", { path: w.path }).catch(() => {});
        refresh();
      });
      const box = el("details", { class: "error-box", open: openErrorBoxes.has(w.path) }, el("summary", {}, `Sorunlu dosyalar (${a.errors.length})`), list, clear);
      box.addEventListener("toggle", () => (box.open ? openErrorBoxes.add(w.path) : openErrorBoxes.delete(w.path)));
      li.append(box);
    }
    ul.append(li);
  }
}

// --- Son yüklemeler ---------------------------------------------------------

const OUTCOME = {
  uploaded: ["ok", "Yüklendi"],
  rejected: ["down", "Yüklenmedi"],
  failed: ["warn", "Gönderilemedi, yeniden denenecek"],
};

function addFeed(data) {
  const [state, label] = OUTCOME[data.outcome] || ["wait", "İşlendi"];
  const ul = $("uploads");
  ul.querySelector(".empty")?.remove();
  ul.prepend(
    el(
      "li",
      { dataset: { state } },
      el("span", { class: "muted" }, new Date().toLocaleTimeString("tr-TR", { timeStyle: "short" })),
      el("span", { class: "file" }, data.name),
      el("span", { class: "muted" }, engagementTitles.get(data.engagement_uuid) || ""),
      el("span", { class: "status", dataset: { state } }, data.outcome === "uploaded" ? label : data.reason || label),
    ),
  );
  while (ul.children.length > FEED_SIZE) ul.lastChild.remove();
}
$("uploads").append(el("li", { class: "empty" }, "Bu oturumda henüz dosya yüklenmedi."));

listen("sidecar://event", (e) => {
  const { event, data } = e.payload;
  if (event === "ready") showReady(data);
  if (event === "upload") addFeed(data);
  if (event === "auth.signed_out" || event === "agent.revoked") {
    engagements = [];
    organizations = [];
  }
  if (["agent.heartbeat", "auth.signed_out", "agent.revoked", "upload"].includes(event)) refresh();
});
listen("sidecar://exit", () => {
  badge($("agent-badge"), "wait", "Yerel ajan yeniden başlatılıyor…");
  adv.sidecar = "Kapandı, yeniden başlatılıyor";
  renderAdv();
});

// --- Muhasebe programı ------------------------------------------------------

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

function srcResult(...nodes) {
  const box = $("src-result");
  box.replaceChildren(...nodes);
  box.hidden = !nodes.length;
}

const DIRECTION = { sales: "Satış", purchase: "Alış", unknown: "Belirsiz" };
const money = (v) => {
  const n = Number(v);
  return v === undefined || v === "" || Number.isNaN(n) ? "" : n.toLocaleString("tr-TR", { minimumFractionDigits: 2, maximumFractionDigits: 2 });
};

$("src-form").addEventListener("submit", async (ev) => {
  ev.preventDefault();
  srcResult(el("p", { class: "muted" }, "Bağlanılıyor…"));
  try {
    const r = await call("source.test", { config: srcConfig() });
    const total = r.tables?.length || 0;
    const missing = r.missing || [];
    const nodes = [
      el(
        "p",
        { class: r.ok ? "ok-text" : "warn-text" },
        r.ok ? "Bağlantı başarılı. Gerekli tabloların hepsi okunabiliyor." : "Bağlantı kuruldu ancak bazı sorunlar var.",
      ),
      el("p", { class: "muted" }, `Beklenen ${total} tablonun ${total - missing.length} tanesi bulundu.`),
    ];
    if (missing.length) {
      nodes.push(el("p", {}, "Bulunamayan ya da okuma izni olmayan tablolar:"), el("ul", { class: "plain" }, ...missing.map((t) => el("li", {}, t))));
    }
    if (r.invoice_query_error) nodes.push(el("p", { class: "warn-text" }, "Fatura sorgusu bu veritabanında çalışmadı. Program sürümü farklı olabilir; destek ekibine bildirin."));
    srcResult(...nodes);
  } catch (e) {
    srcResult(el("p", { class: "warn-text" }, errText(e)));
  }
});

$("src-preview").addEventListener("click", async () => {
  const now = new Date();
  const from = new Date(Date.UTC(now.getUTCFullYear(), now.getUTCMonth() - 1, 1));
  const to = new Date(Date.UTC(now.getUTCFullYear(), now.getUTCMonth(), 1));
  const day = (d) => d.toISOString().slice(0, 10);
  srcResult(el("p", { class: "muted" }, "Okunuyor…"));
  try {
    const r = await call("source.preview", { config: srcConfig(), from: day(from), to: day(to) });
    const dir = r.by_direction || {};
    const month = from.toLocaleDateString("tr-TR", { month: "long", year: "numeric", timeZone: "UTC" });
    const nodes = [
      el("p", {}, `${month}: ${r.total} fatura okundu (satış ${dir.sales || 0}, alış ${dir.purchase || 0}${dir.unknown ? `, türü belirsiz ${dir.unknown}` : ""}).`),
    ];
    if (r.truncated) nodes.push(el("p", { class: "warn-text" }, "Okuma sınırına ulaşıldı; liste eksik olabilir."));
    const warnCount = Object.values(r.warning_counts || {}).reduce((a, b) => a + b, 0);
    if (warnCount) nodes.push(el("p", { class: "warn-text" }, `${warnCount} faturada denetlenmesi gereken alan var.`));
    const rows = (r.sample || []).slice(0, 10);
    if (rows.length) {
      const head = el("tr", {}, ...["Belge no", "Tarih", "Tür", "Karşı taraf", "Tutar"].map((h) => el("th", {}, h)));
      const body = rows.map((inv) =>
        el(
          "tr",
          {},
          el("td", {}, inv.doc_no || inv.internal_no || ""),
          el("td", {}, inv.date ? new Date(inv.date).toLocaleDateString("tr-TR", { timeZone: "UTC" }) : ""),
          el("td", {}, (DIRECTION[inv.direction] || "") + (inv.cancelled ? " (iptal)" : "")),
          el("td", {}, inv.counterparty_title || ""),
          el("td", { class: "num" }, money(inv.totals?.net || inv.totals?.amount)),
        ),
      );
      nodes.push(el("table", { class: "grid" }, el("thead", {}, head), el("tbody", {}, ...body)));
      if (r.total > rows.length) nodes.push(el("p", { class: "muted" }, `İlk ${rows.length} fatura gösteriliyor.`));
    }
    srcResult(...nodes);
  } catch (e) {
    srcResult(el("p", { class: "warn-text" }, errText(e)));
  }
});

// --- Ayarlar ----------------------------------------------------------------

let paused = false;

async function loadSettings() {
  try {
    const s = await invoke("shell_settings");
    $("set-autostart").checked = s.autostart;
    setPaused(s.paused);
  } catch {
    /* kabuk ayarları okunamazsa kutular olduğu gibi kalır */
  }
}

function setPaused(p) {
  paused = !!p;
  $("set-paused").checked = paused;
  $("paused-banner").hidden = !paused;
}

$("set-autostart").addEventListener("change", async (e) => {
  try {
    e.target.checked = await invoke("set_autostart_enabled", { enabled: e.target.checked });
    show($("settings-msg"), "");
  } catch {
    e.target.checked = !e.target.checked;
    show($("settings-msg"), "Otomatik başlatma ayarlanamadı.");
  }
});

async function changePaused(p) {
  try {
    setPaused(await invoke("set_watch_paused", { paused: p }));
    show($("settings-msg"), "");
  } catch {
    show($("settings-msg"), "İzleme durumu değiştirilemedi.");
  }
  refresh();
}
$("set-paused").addEventListener("change", (e) => changePaused(e.target.checked));
$("paused-resume").addEventListener("click", () => changePaused(false));
listen("shell://paused", (e) => setPaused(e.payload));

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
  adv.app = u.current;
  renderAdv();
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

// --- Gelişmiş ---------------------------------------------------------------

const adv = { app: "", sidecar: "", platform: "", pid: "" };

function renderAdv() {
  fill($("adv-info"), [
    ["Uygulama sürümü", adv.app],
    ["Yerel ajan", adv.sidecar],
    ["Platform", adv.platform],
    ["Süreç no", adv.pid],
    ["Günlük dosyası", "Uygulama günlük klasöründe hakan.log"],
    ["Son teknik hata", lastTechnicalError],
  ]);
}

function showReady(info) {
  adv.sidecar = `Çalışıyor (sürüm ${info.version})`;
  adv.platform = `${info.os} / ${info.arch}`;
  adv.pid = info.pid;
  renderAdv();
  refresh();
}

$("adv-ping").addEventListener("click", async () => {
  const t0 = performance.now();
  try {
    await call("ping", { echo: "merhaba" });
    $("adv-out").textContent = `Yerel ajan yanıt verdi (${(performance.now() - t0).toFixed(0)} ms).`;
  } catch (e) {
    $("adv-out").textContent = errText(e);
  }
  renderAdv();
});

$("adv-restart").addEventListener("click", async () => {
  $("adv-out").textContent = "Yeniden başlatılıyor…";
  try {
    await invoke("sidecar_restart");
    setPaused(false);
  } catch (e) {
    $("adv-out").textContent = errText(e);
  }
});

// Sürükle-bırak: .xml → yerel denetim (Gelişmiş); klasör → klasör ekleme.
listen("tauri://drag-drop", async (e) => {
  for (const path of e.payload.paths || []) {
    if (path.toLowerCase().endsWith(".xml")) {
      try {
        const s = await call("ubl.inspect", { path });
        fill($("ubl-out"), [
          ["Dosya", folderName(path)],
          ["Fatura no", s.id],
          ["Senaryo / tip", [s.profile, s.type].filter(Boolean).join(" / ")],
          ["Satıcı", s.supplier],
          ["Alıcı", s.customer],
          ["Ödenecek", s.payable ? `${money(s.payable)} ${s.currency || ""}`.trim() : ""],
        ]);
      } catch (err) {
        fill($("ubl-out"), [["Sonuç", "Dosya e-fatura (UBL-TR) olarak okunamadı."]]);
        errText(err);
      }
      currentTab = "advanced";
      setView("advanced");
    } else if (!/\.[a-z0-9]{2,4}$/i.test(path) && status?.running) {
      currentTab = "folders";
      setView("folders");
      openAdd();
      setPicked(path);
    }
  }
});

// --- Başlangıç --------------------------------------------------------------

window.__TAURI__.app
  .getVersion()
  .then((v) => {
    adv.app = v;
    renderAdv();
  })
  .catch(() => {});
invoke("sidecar_status")
  .then((s) => s.ready && showReady(s.ready))
  .catch(() => {});
invoke("update_status").then(showUpdate).catch(() => {});
loadSettings();
refresh();
setInterval(refresh, 15000);

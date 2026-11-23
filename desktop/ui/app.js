// Hakan masaüstü arayüzü, arayüz v1 (#325; ilk sürüm #206). Ön yüz yalnızca
// kabuk komutlarını çağırır; sidecar'a doğrudan erişmez, oturum belirteci ön
// yüze hiç gelmez. Kabuk ve sidecar davranışı değişmedi; yalnızca arayüz yeni.
//
// Kural: hiçbir ekranda UUID, iç kod ya da ham JSON gösterilmez. Mükellefler
// adlarıyla, hatalar Türkçe açıklamalarıyla görünür. Tanılama araçları
// Ayarlar → Gelişmiş bölümündedir.
import { degerlendir, yeniDurum } from "./bildirim.js";
import { BosDurum, HataDurumu, TacRozeti } from "./hakan-ui.js";
import { temaSec, temaTercihi } from "./tema.js";

const { invoke } = window.__TAURI__.core;
const { listen } = window.__TAURI__.event;

const $ = (id) => document.getElementById(id);
const call = (method, params = {}) => invoke("sidecar_call", { method, params });
const HISTORY_SIZE = 200;
const SCAN_SECONDS = 15; // sidecar/agent/runtime.go ScanInterval

// --- Yerel depo (yalnızca bu bilgisayarda; hata verirse sessizce geçilir) ---

const store = {
  get(key, fallback) {
    try {
      const v = localStorage.getItem(key);
      return v == null ? fallback : JSON.parse(v);
    } catch {
      return fallback;
    }
  },
  set(key, value) {
    try {
      localStorage.setItem(key, JSON.stringify(value));
    } catch {
      /* depo kapalı */
    }
  },
};

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

const GENERIC_ERROR = "Beklenmeyen bir sorun oluştu. Sorun sürerse Ayarlar → Gelişmiş bölümündeki bilgileri destek ekibine iletin.";
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

const validDate = (iso) => {
  if (!iso) return null;
  const d = new Date(iso);
  return Number.isNaN(d.getTime()) || d.getFullYear() < 2000 ? null : d;
};

const fmtTime = (iso) => validDate(iso)?.toLocaleString("tr-TR", { dateStyle: "short", timeStyle: "short" }) ?? "";

function relTime(iso) {
  const d = validDate(iso);
  if (!d) return "";
  const s = (Date.now() - d.getTime()) / 1000;
  if (s < 60) return "az önce";
  if (s < 3600) return `${Math.floor(s / 60)} dk önce`;
  if (s < 86400) return `${Math.floor(s / 3600)} sa önce`;
  if (s < 2 * 86400) return "dün";
  return fmtTime(iso);
}

/** Geçmiş tablosu için kısa zaman: bugün saat, dün, ya da tarih. */
function shortTime(iso) {
  const d = validDate(iso);
  if (!d) return "";
  const now = new Date();
  if (d.toDateString() === now.toDateString()) return d.toLocaleTimeString("tr-TR", { timeStyle: "short" });
  const y = new Date(now);
  y.setDate(now.getDate() - 1);
  if (d.toDateString() === y.toDateString()) return "dün";
  return d.toLocaleDateString("tr-TR", { day: "numeric", month: "short" });
}

const folderName = (p) => (p || "").split(/[\\/]/).filter(Boolean).pop() || p;

function show(node, text, kind) {
  node.textContent = text || "";
  node.hidden = !text;
  if (kind) node.dataset.kind = kind;
  else delete node.dataset.kind;
}

function fill(dl, rows) {
  dl.replaceChildren();
  for (const [k, v] of rows) {
    if (v === undefined || v === null || v === "") continue;
    dl.append(el("dt", {}, k), el("dd", {}, String(v)));
  }
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

const SVG_NS = "http://www.w3.org/2000/svg";
function icon(d, cls = "ic") {
  const svg = document.createElementNS(SVG_NS, "svg");
  svg.setAttribute("class", cls);
  svg.setAttribute("viewBox", "0 0 24 24");
  svg.setAttribute("aria-hidden", "true");
  const p = document.createElementNS(SVG_NS, "path");
  p.setAttribute("d", d);
  svg.append(p);
  return svg;
}
const IC = {
  folder: "M3 6h6l2 2h10v11H3z",
  more: "M5 12h.01M12 12h.01M19 12h.01",
  check: "M4 12.5l5 5L20 6.5",
};

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

const SETUP_VIEWS = ["loading", "login", "office", "first"];
// Kenar menü (tek program): Ofis paneli | Klasörler | Geçmiş | Sorunlar | Ayarlar.
const TABS = ["panel", "folders", "history", "problems", "settings"];
// Sekmesi olmayan alt görünümler ve menüde seçili görünen sekmeleri.
const SUBVIEWS = { program: "settings" };
const VIEWS = [...TABS, ...Object.keys(SUBVIEWS)];
let currentTab = "folders";
let currentView = "loading";

function setView(name) {
  currentView = name;
  const inApp = VIEWS.includes(name);
  $("setup").hidden = inApp;
  $("app").hidden = !inApp;
  for (const v of SETUP_VIEWS) $(`view-${v}`).hidden = v !== name;
  for (const v of VIEWS) $(`view-${v}`).hidden = v !== name;
  const tab = SUBVIEWS[name] || name;
  for (const b of $("tabs").querySelectorAll("[role=tab]")) {
    const on = b.dataset.view === tab;
    b.setAttribute("aria-selected", String(on));
    b.tabIndex = on || (!inApp && b.dataset.view === "folders") ? 0 : -1;
  }
  if (name === "problems") startProblemTicker();
  else stopProblemTicker();
}

function selectTab(v, focus = false) {
  currentTab = SUBVIEWS[v] || v;
  setView(v);
  if (v === "settings") loadSettings();
  if (v === "history") renderHistory();
  if (v === "panel") loadOffice();
  if (focus) $("tabs").querySelector(`[data-view="${currentTab}"]`)?.focus();
}

$("tabs").addEventListener("click", (e) => {
  const v = e.target.closest("[role=tab]")?.dataset.view;
  if (v) selectTab(v);
});
// Sekme listesi: ok tuşlarıyla gezinme (WAI-ARIA tabs).
$("tabs").addEventListener("keydown", (e) => {
  const i = TABS.indexOf(currentTab);
  const next = { ArrowDown: 1, ArrowRight: 1, ArrowUp: -1, ArrowLeft: -1 }[e.key];
  if (next !== undefined) {
    e.preventDefault();
    selectTab(TABS[(i + next + TABS.length) % TABS.length], true);
  } else if (e.key === "Home" || e.key === "End") {
    e.preventDefault();
    selectTab(e.key === "Home" ? TABS[0] : TABS.at(-1), true);
  }
});

// --- Ajan durumu ------------------------------------------------------------

let status = null;
const engagementTitles = new Map(); // engagement → mükellef adı
let engagements = [];
let firstRunSkipped = store.get("hakan.kurulum.atlandi", false);
let appVersion = "";

function agentState(st) {
  if (paused) return ["paused", "Duraklatıldı"];
  if (st.heartbeat_error) return ["down", "Bağlantı sorunu"];
  if (problemCount(st) > 0) return ["wait", "Sorun var"];
  return ["ok", "Bağlı"];
}

async function refresh() {
  let st;
  try {
    st = await call("agent.status");
  } catch (e) {
    if (currentView === "loading" || e?.code === "SIDECAR_DOWN") setAgent("wait", "Yerel ajan başlatılıyor…");
    return;
  }
  status = st;
  if (st.base_url && !$("api-url").value) $("api-url").value = st.base_url;
  if (!st.base_url) $("server-box").open = true;

  const running = st.signed_in && st.running;
  $("office-name").textContent = running ? st.organization?.name || "" : "";
  if (!st.signed_in) {
    setAgent("wait", "Oturum açılmadı");
    if (currentView !== "login") setView("login");
    return;
  }
  if (!running) {
    setAgent("wait", "Ofis seçilmedi");
    if (currentView !== "office") {
      setView("office");
      loadOffices();
    }
    return;
  }
  if (!engagements.length) await loadEngagements();

  // Kurulumun 3. adımı: ilk klasör (yalnızca hiç klasör yokken ve atlanmadıysa).
  if (!st.watches.length && !firstRunSkipped && !VIEWS.includes(currentView)) {
    if (currentView !== "first") openFirst();
    return;
  }
  if (!VIEWS.includes(currentView)) setView(currentTab);

  const [state, label] = agentState(st);
  setAgent(state, label);
  $("agent-meta").textContent = [st.agent_name, appVersion].filter(Boolean).join(" · ");
  fill($("account-info"), [
    ["Ofis", st.organization?.name],
    ["Bu bilgisayar", st.agent_name],
    ["Sunucu", st.base_url],
    ["Son eşitleme", relTime(st.last_heartbeat)],
  ]);
  renderWatches(st.watches);
  renderTargets(st);
  renderProblems(st);
  notify(st);
}

function setAgent(state, text) {
  $("agent-dot").dataset.state = state;
  $("agent-badge").textContent = text;
  $("agent-card").setAttribute("aria-label", `Ajan durumu: ${text}`);
}

// --- Kurulum 1 · Giriş --------------------------------------------------------

$("api-check").addEventListener("click", async () => {
  show($("login-msg"), "Deneniyor…", "info");
  try {
    await call("api.health", { base_url: $("api-url").value.trim() });
    show($("login-msg"), "Sunucuya bağlanıldı.", "ok");
  } catch (e) {
    show($("login-msg"), errText(e));
  }
});

let organizations = [];

$("login-form").addEventListener("submit", async (ev) => {
  ev.preventDefault();
  const email = $("email").value.trim();
  const password = $("password").value;
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
    firstRunSkipped = false;
    store.set("hakan.kurulum.atlandi", false);
    renderOffices();
  } catch (e) {
    if (e?.code === "MFA_REQUIRED") {
      $("totp-row").hidden = false;
      $("totp").focus();
      show($("login-msg"), "Hesabınızda iki adımlı doğrulama açık. Doğrulama kodunu girin.", "info");
    } else {
      show($("login-msg"), errText(e));
    }
  } finally {
    $("login-submit").disabled = false;
  }
  refresh();
});

// --- Kurulum 2 · Ofis ve bilgisayar adı ----------------------------------------

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

const initials = (name) =>
  (name || "")
    .split(/\s+/)
    .filter(Boolean)
    .slice(0, 2)
    .map((w) => w[0].toLocaleUpperCase("tr"))
    .join("");

function renderOffices() {
  const box = $("office-list");
  box.replaceChildren();
  organizations.forEach((o, i) => {
    box.append(
      el(
        "label",
        { class: "office" },
        el("input", { type: "radio", name: "office", value: o.slug, checked: i === 0, "aria-label": o.name }),
        el("span", { class: "av", "aria-hidden": "true" }, initials(o.name)),
        el("b", { class: "grow t-sm" }, o.name),
        icon(IC.check, "ic check"),
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
  $("first-taxpayers").replaceChildren(...engagements.map((e) => el("option", { value: e.title })));
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
const engagementByTitle = (title) => engagements.find((e) => e.title.toLocaleLowerCase("tr") === title.trim().toLocaleLowerCase("tr"));

// --- Kurulum 3 · İlk klasör ------------------------------------------------------

let firstFolder = "";

function openFirst() {
  setView("first");
  setFirstPicked("");
  show($("first-msg"), "");
}

function setFirstPicked(path) {
  firstFolder = path || "";
  $("first-picked").textContent = firstFolder || "Klasör seç ya da pencereye sürükle";
  $("first-picked").className = firstFolder ? "grow mono t-xs" : "grow t-sm t-2";
  if (firstFolder) $("first-pick").dataset.picked = "";
  else delete $("first-pick").dataset.picked;
}

$("first-pick").addEventListener("click", async () => {
  try {
    const p = await invoke("pick_folder");
    if (p) setFirstPicked(p);
  } catch (e) {
    show($("first-msg"), errText(e));
  }
});

$("first-form").addEventListener("submit", async (ev) => {
  ev.preventDefault();
  const eng = engagementByTitle($("first-taxpayer").value);
  if (!eng) return show($("first-msg"), "Listeden bir mükellef seçin.");
  if (!firstFolder) return show($("first-msg"), "İzlenecek klasörü seçin.");
  try {
    await call("watch.add", { path: firstFolder, engagement_uuid: eng.id, label: "", taxpayer_title: eng.title });
    finishSetup();
  } catch (e) {
    show($("first-msg"), errText(e));
  }
});

$("first-skip").addEventListener("click", finishSetup);

function finishSetup() {
  firstRunSkipped = true;
  store.set("hakan.kurulum.atlandi", true);
  currentTab = "folders";
  setView("folders");
  refresh();
}

// --- Klasörler --------------------------------------------------------------

let pickedFolder = "";

function setPicked(path) {
  pickedFolder = path || "";
  $("picked-folder").textContent = pickedFolder || "Klasör seç… ya da pencereye sürükle";
  $("picked-folder").className = pickedFolder ? "grow mono t-xs" : "grow t-sm t-2";
  if (pickedFolder) $("pick-folder").dataset.picked = "";
  else delete $("pick-folder").dataset.picked;
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

/** Klasör satırının durumu: [pill durumu, metin]. */
function watchState(a) {
  if (a.folder_error) return ["down", "Klasör bulunamadı"];
  if (paused) return ["", "Duraklatıldı"];
  if (a.errors?.length) return ["warn", `${a.errors.length} dosyada sorun`];
  return ["ok", "İzleniyor"];
}

async function removeWatch(w) {
  if (!(await confirmBox(`“${w.label || folderName(w.path)}” klasörü izlemeden çıkarılsın mı? Klasördeki dosyalara dokunulmaz.`, "İzlemeden çıkar"))) return;
  await call("watch.remove", { path: w.path }).catch(() => {});
  refresh();
}

function renderWatches(watches) {
  $("cnt-folders").textContent = watches.length ? String(watches.length) : "";
  const list = $("watches");
  list.replaceChildren();
  list.hidden = !watches.length;
  $("watches-empty").hidden = !!watches.length;
  if (!watches.length) {
    const add = el("button", { type: "button", class: "btn btn-pri" }, "+ Klasör ekle");
    add.addEventListener("click", openAdd);
    $("watches-empty").replaceChildren(
      el("div", { class: "card" }, BosDurum({ baslik: "Henüz izlenen klasör yok", aciklama: "e-Fatura XML ya da ZIP'lerinin düştüğü klasörü ekle; dosyalar seçtiğin mükellefe kendiliğinden gönderilir.", eylem: add })),
    );
    return;
  }
  for (const w of watches) {
    const a = w.activity || { errors: [] };
    const [state, text] = watchState(a);
    const sync = a.last_upload_at ? `${relTime(a.last_upload_at)} · ${a.uploaded || 0}` : "—";
    const menu = el(
      "details",
      { class: "menu" },
      el("summary", { class: "btn btn-sm btn-icon btn-ghost", role: "button", "aria-label": `${taxpayerOf(w)}: seçenekler` }, icon(IC.more, "ic ic-sm")),
      el(
        "div",
        { class: "menu-pop" },
        a.errors?.length ? el("button", { type: "button", class: "btn btn-sm btn-ghost", onclick: () => selectTab("problems") }, "Sorunları göster") : null,
        el("button", { type: "button", class: "btn btn-sm btn-ghost danger-text", onclick: () => removeWatch(w) }, "İzlemeden çıkar"),
      ),
    );
    list.append(
      el(
        "div",
        { class: "fl", role: "listitem", dataset: { state } },
        el("span", { class: "fl-ic" }, icon(IC.folder)),
        el(
          "div",
          { class: "col grow" },
          el("span", { class: "fl-title" }, w.label ? `${taxpayerOf(w)} · ${w.label}` : taxpayerOf(w)),
          el("span", { class: "fl-path", title: w.path }, w.path),
        ),
        el("span", { class: "fl-sync num", title: "Son gönderim · bu oturumda gönderilen" }, sync),
        el("span", { class: "fl-state" }, el("span", { class: "pill", dataset: { state } }, text)),
        menu,
      ),
    );
  }
}

// --- Geçmiş -----------------------------------------------------------------

// Bu bilgisayarda tutulur (son 200 dosya); sidecar geçmiş tutmaz.
let history = store.get("hakan.gecmis", []);

const OUTCOME = {
  uploaded: { durum: "yuklendi", state: "ok", label: "Eklendi" },
  duplicate: { durum: "yuklendi", state: "", label: "Zaten var" },
  rejected: { durum: "sorun", state: "down", label: "Gönderilmedi" },
  failed: { durum: "sorun", state: "down", label: "Yeniden denenecek" },
};

function addHistory(data) {
  history.unshift({
    at: new Date().toISOString(),
    name: data.name,
    taxpayer: engagementTitles.get(data.engagement_uuid) || "",
    outcome: data.outcome,
    reason: data.outcome === "uploaded" || data.outcome === "duplicate" ? "" : data.reason || "",
  });
  history = history.slice(0, HISTORY_SIZE);
  store.set("hakan.gecmis", history);
  renderHistory();
}

function renderHistory() {
  const now = Date.now();
  const today = new Date().toDateString();
  const sent = history.filter((h) => h.outcome === "uploaded");
  const todayCount = sent.filter((h) => new Date(h.at).toDateString() === today).length;
  const weekCount = sent.filter((h) => now - new Date(h.at).getTime() < 7 * 86400000).length;
  $("history-sub").textContent = `Bugün ${todayCount} dosya · son 7 gün ${weekCount}`;
  $("cnt-history").textContent = "";

  const q = $("history-search").value.trim().toLocaleLowerCase("tr");
  const rows = history.filter((h) => !q || `${h.name} ${h.taxpayer}`.toLocaleLowerCase("tr").includes(q));
  const body = $("uploads");
  body.replaceChildren(
    ...rows.map((h) => {
      const o = OUTCOME[h.outcome] || { durum: "bekleniyor", state: "", label: "İşlendi" };
      return el(
        "tr",
        {},
        el("td", {}, TacRozeti({ durum: o.durum, metin: o.label })),
        el("td", { class: "file", title: h.name }, h.name),
        el("td", { class: "t-sm" }, h.taxpayer),
        el("td", {}, el("span", { class: "pill", dataset: { state: o.state } }, h.reason || o.label)),
        el("td", { class: "t-xs t-3 num" }, shortTime(h.at)),
      );
    }),
  );
  $("history-table").hidden = !rows.length;
  $("history-empty").hidden = !!rows.length;
  if (!rows.length) {
    $("history-empty").replaceChildren(
      BosDurum(
        q
          ? { baslik: "Eşleşen dosya yok", aciklama: "Aramayı değiştir ya da temizle." }
          : { baslik: "Henüz gönderilen dosya yok", aciklama: "İzlenen klasörlere düşen e-Faturalar burada listelenir." },
      ),
    );
  }
}
$("history-search").addEventListener("input", renderHistory);

// --- Sorunlar ---------------------------------------------------------------

// "Yok say" yalnızca bu listeden gizler (dosya ve klasör değişmez).
let ignored = new Set(store.get("hakan.yoksay", []));
const errKey = (w, e) => `${w.path}|${e.name}|${e.at}`;

function collectProblems(st) {
  const transient = [];
  const permanent = [];
  const folders = [];
  for (const w of st?.watches || []) {
    const a = w.activity || { errors: [] };
    if (a.folder_error) folders.push({ w, message: a.folder_error });
    for (const e of a.errors || []) {
      if (ignored.has(errKey(w, e))) continue;
      (e.retry ? transient : permanent).push({ w, e });
    }
  }
  return { transient, permanent, folders };
}

function problemCount(st) {
  const p = collectProblems(st);
  return p.transient.length + p.permanent.length + p.folders.length;
}

function problemCard(durum, title, text, ...actions) {
  return el(
    "div",
    { class: "card problem" },
    TacRozeti({ durum, metin: durum === "sorun" ? "Sorun var" : "Bilgi" }),
    el("div", { class: "col grow gap-6" }, el("b", {}, title), text),
    ...actions,
  );
}

function nextScanFraction(iso) {
  const d = validDate(iso);
  if (!d) return 0;
  const s = ((Date.now() - d.getTime()) / 1000) % SCAN_SECONDS;
  return Math.max(0, Math.min(1, s / SCAN_SECONDS));
}

function renderProblems(st) {
  const { transient, permanent, folders } = collectProblems(st);
  const total = transient.length + permanent.length + folders.length;
  $("cnt-problems").textContent = total ? String(total) : "";
  const box = $("problems");
  box.replaceChildren();

  // #332: sunucuya ulaşılamıyorsa önce bu söylenir; dosyalar bilgisayarda
  // bekler, bağlantı gelince gönderilir (veri güvende).
  if (st.heartbeat_error) {
    const tekrar = el("button", { type: "button", class: "btn btn-sm", onclick: () => refresh() }, "Yeniden dene");
    box.append(
      el(
        "div",
        { class: "card" },
        HataDurumu({
          aciklama: "Dosyalar bu bilgisayarda bekliyor; bağlantı gelince kendiliğinden gönderilecek.",
          verilerGuvende: true,
          kod: String(st.heartbeat_error).slice(0, 120),
          eylem: tekrar,
        }),
      ),
    );
  }

  if (!total && !dupToday().length) {
    if (!st.heartbeat_error) box.append(el("div", { class: "card" }, BosDurum({ baslik: "Sorun yok, taç tamam", aciklama: "Gönderilemeyen dosya olursa burada görünür." })));
    return;
  }

  // Geçici: ağ / sunucu (kendiliğinden yeniden denenir)
  if (transient.length) {
    const latest = transient.map((t) => t.e.at).sort().at(-1);
    const left = Math.ceil(SCAN_SECONDS * (1 - nextScanFraction(latest)));
    const bar = el("div", { class: "bar warn", role: "progressbar", "aria-label": "Sonraki deneme", "aria-valuemin": "0", "aria-valuemax": "100" }, el("i", { id: "retry-bar" }));
    const reasons = [...new Set(transient.map((t) => t.e.message).filter(Boolean))];
    box.append(
      el("span", { class: "t-cap" }, "Geçici · kendiliğinden yeniden denenir"),
      problemCard(
        "sorun",
        `${transient.length} dosya · ${reasons.length === 1 ? reasons[0].replace(/[.;].*$/, "").toLocaleLowerCase("tr") : "sunucuya ulaşılamadı"}`,
        el(
          "div",
          { class: "col gap-6" },
          el("span", { class: "t-sm t-2" }, transient.map((t) => t.e.name).join(", ")),
          el("span", { class: "t-sm t-2", id: "retry-text" }, `Sonraki taramada otomatik yeniden denenecek (yaklaşık ${left} sn).`),
          bar,
        ),
      ),
    );
    updateRetryBar(latest);
  }

  // Kalıcı: kullanıcı kararı gerekir
  if (permanent.length || folders.length) {
    box.append(el("span", { class: "t-cap" }, "Kalıcı · senin kararını bekler"));
    for (const { w, message } of folders) {
      box.append(
        problemCard(
          "sorun",
          `${taxpayerOf(w)} · klasör bulunamadı`,
          el("span", { class: "t-sm t-2" }, `${message.charAt(0).toUpperCase()}${message.slice(1)}. Klasör taşındıysa izlemeden çıkarıp yeniden ekle.`),
          el("button", { type: "button", class: "btn btn-sm", onclick: () => removeWatch(w) }, "İzlemeden çıkar"),
        ),
      );
    }
    for (const { w, e } of permanent) {
      box.append(
        problemCard(
          "sorun",
          `${e.name} · ${taxpayerOf(w)}`,
          el("span", { class: "t-sm t-2" }, `${e.message || "Dosya gönderilmedi."} ${fmtTime(e.at)}`.trim()),
          el(
            "button",
            {
              type: "button",
              class: "btn btn-sm btn-ghost",
              onclick: () => {
                ignored.add(errKey(w, e));
                store.set("hakan.yoksay", [...ignored].slice(-500));
                renderProblems(status);
                renderStatusOnly();
              },
            },
            "Yok say",
          ),
        ),
      );
    }
  }

  // Bilgi: mükerrer (işlem gerekmez)
  const dups = dupToday();
  if (dups.length) {
    box.append(
      el("span", { class: "t-cap" }, "Bilgi"),
      problemCard("yuklendi", `${dups.length} dosya · zaten var`, el("span", { class: "t-sm t-2" }, "Aynı dosya daha önce gönderilmiş (SHA-256). İşlem gerekmez.")),
    );
  }
}

function dupToday() {
  const today = new Date().toDateString();
  return history.filter((h) => h.outcome === "duplicate" && new Date(h.at).toDateString() === today);
}

function renderStatusOnly() {
  if (!status) return;
  const [s, l] = agentState(status);
  setAgent(s, l);
}

let retryLatest = null;
function updateRetryBar(latest = retryLatest) {
  retryLatest = latest;
  const bar = $("retry-bar");
  if (!bar || !latest) return;
  const f = nextScanFraction(latest);
  bar.style.width = `${Math.round(f * 100)}%`; // CSSOM; CSP satır içi stil özniteliğini engeller, bunu değil
  bar.parentElement.setAttribute("aria-valuenow", String(Math.round(f * 100)));
  const t = $("retry-text");
  if (t) t.textContent = `Sonraki taramada otomatik yeniden denenecek (yaklaşık ${Math.ceil(SCAN_SECONDS * (1 - f))} sn).`;
}
let ticker = null;
function startProblemTicker() {
  if (!ticker) ticker = setInterval(() => updateRetryBar(), 1000);
}
function stopProblemTicker() {
  clearInterval(ticker);
  ticker = null;
}

// --- Bildirimler (kurallar: bildirim.js) ---------------------------------------

const notifyState = Object.assign(yeniDurum(), store.get("hakan.bildirim", {}));

function notify(st) {
  const { permanent } = collectProblems(st);
  const out = degerlendir(notifyState, {
    agSorunu: !!st.heartbeat_error || collectProblems(st).transient.length > 0,
    kaliciHatalar: permanent.map(({ w, e }) => ({ anahtar: errKey(w, e) })),
  });
  store.set("hakan.bildirim", notifyState);
  for (const n of out) systemNotification(n.baslik, n.govde);
}

async function systemNotification(title, body) {
  if (!("Notification" in window)) return;
  try {
    if (Notification.permission === "default") await Notification.requestPermission();
    if (Notification.permission === "granted") new Notification(title, { body });
  } catch {
    /* bildirim gösterilemezse sessizce geç */
  }
}

// --- Olaylar -------------------------------------------------------------------

listen("sidecar://event", (e) => {
  const { event, data } = e.payload;
  if (event === "ready") showReady(data);
  if (event === "upload") addHistory(data);
  if (event === "auth.signed_out" || event === "agent.revoked") {
    engagements = [];
    organizations = [];
  }
  if (["agent.heartbeat", "auth.signed_out", "agent.revoked", "upload", "transfer"].includes(event)) refresh();
});
listen("sidecar://exit", () => {
  setAgent("wait", "Yerel ajan yeniden başlatılıyor…");
  adv.sidecar = "Kapandı, yeniden başlatılıyor";
  renderAdv();
});

// --- Muhasebe programı ------------------------------------------------------

// Luca / Zirve aktarım klasörleri (#391). Sidecar dosyaları bırakır ve
// sonucu sunucuya bildirir; ekranda yalnızca mükellef, program, klasör ve
// Türkçe sonuç görünür.
const XFER_PROGRAM = { luca: "Luca", zirve: "Zirve" };
const XFER_OUTCOME = {
  delivered: { state: "ok", label: "Klasöre bırakıldı" },
  failed: { state: "down", label: "Bırakılamadı" },
  retry: { state: "warn", label: "Yeniden denenecek" },
};
let xferFolder = "";

function setXferPicked(path) {
  xferFolder = path || "";
  $("xfer-picked").textContent = xferFolder || "Aktarım klasörünü seç…";
  $("xfer-picked").className = xferFolder ? "grow mono t-xs" : "grow t-sm t-2";
}

function renderXferEngagements() {
  const sel = $("xfer-engagement");
  const keep = sel.value;
  sel.replaceChildren(...engagements.map((e) => el("option", { value: e.id }, e.title)));
  if (keep) sel.value = keep;
}

async function removeTarget(t) {
  const name = `${taxpayerOf(t)} · ${XFER_PROGRAM[t.driver] || t.driver}`;
  if (!(await confirmBox(`${name} aktarım klasörü kaldırılsın mı? Klasördeki dosyalara dokunulmaz.`, "Kaldır"))) return;
  await call("transfer.remove_target", { engagement_uuid: t.engagement_uuid, driver: t.driver }).catch(() => {});
  refresh();
}

function renderTargets(st) {
  const targets = st.transfer_targets || [];
  if ($("xfer-engagement").options.length !== engagements.length) renderXferEngagements();
  $("xfer-empty").hidden = !!targets.length;
  $("xfer-list").replaceChildren(
    ...targets.map((t) =>
      el(
        "div",
        { class: "fl", role: "listitem" },
        el("span", { class: "fl-ic" }, icon(IC.folder)),
        el(
          "div",
          { class: "col grow" },
          el("span", { class: "fl-title" }, `${taxpayerOf(t)} · ${XFER_PROGRAM[t.driver] || t.driver}`),
          el("span", { class: "fl-path", title: t.path }, t.path),
        ),
        el("button", { type: "button", class: "btn btn-sm btn-ghost danger-text", onclick: () => removeTarget(t) }, "Kaldır"),
      ),
    ),
  );
  const tr = st.transfer || {};
  const recent = tr.recent || [];
  $("xfer-recent-box").hidden = !recent.length && !tr.last_error;
  const rows = recent.map((ev) => {
    const o = XFER_OUTCOME[ev.outcome] || XFER_OUTCOME.retry;
    const what = [taxpayerOf(ev), XFER_PROGRAM[ev.driver] || ev.driver, ev.period].filter(Boolean).join(" · ");
    const detail = ev.outcome === "delivered" ? `${ev.folder} klasörüne: ${(ev.files || []).join(", ")}` : ev.reason || "";
    return el(
      "div",
      { class: "row gap-8", role: "listitem" },
      el("span", { class: "pill", dataset: { state: o.state } }, o.label),
      el("div", { class: "col grow" }, el("span", { class: "t-sm" }, what), el("span", { class: "t-xs t-2" }, detail)),
      el("span", { class: "t-xs t-3 num" }, relTime(ev.at)),
    );
  });
  if (tr.last_error) rows.unshift(el("p", { class: "warn-text" }, tr.last_error));
  $("xfer-recent").replaceChildren(...rows);
}

$("xfer-pick").addEventListener("click", async () => {
  try {
    const p = await invoke("pick_folder");
    if (p) setXferPicked(p);
  } catch (e) {
    show($("xfer-msg"), errText(e));
  }
});

$("xfer-form").addEventListener("submit", async (ev) => {
  ev.preventDefault();
  const id = $("xfer-engagement").value;
  const driver = document.querySelector("input[name=xfer-driver]:checked")?.value || "luca";
  if (!id) return show($("xfer-msg"), "Bir mükellef seçin.");
  if (!xferFolder) return show($("xfer-msg"), "Aktarım klasörünü seçin.");
  try {
    await call("transfer.add_target", { path: xferFolder, engagement_uuid: id, driver, taxpayer_title: engagementTitles.get(id) || "" });
    setXferPicked("");
    show($("xfer-msg"), "");
  } catch (e) {
    show($("xfer-msg"), errText(e));
  }
  refresh();
});

const srcKind = () => document.querySelector("input[name=src-kind]:checked")?.value || "logo_tiger";

function srcConfig() {
  const kind = srcKind();
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

for (const r of document.querySelectorAll("input[name=src-kind]")) {
  r.addEventListener("change", () => {
    for (const f of document.querySelectorAll(".logo-only")) f.hidden = srcKind() === "mikro";
  });
}

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
  srcResult(el("p", { class: "t-2" }, "Bağlanılıyor…"));
  try {
    const r = await call("source.test", { config: srcConfig() });
    const total = r.tables?.length || 0;
    const missing = r.missing || [];
    const nodes = [
      r.ok
        ? el("div", { class: "ok-box" }, icon(IC.check), el("span", { class: "t-sm" }, "Bağlantı başarılı · gerekli tabloların hepsi okunabiliyor"))
        : el("p", { class: "warn-text" }, "Bağlantı kuruldu ancak bazı sorunlar var."),
      el("p", { class: "t-2" }, `Beklenen ${total} tablonun ${total - missing.length} tanesi bulundu.`),
    ];
    if (missing.length) {
      nodes.push(el("p", {}, "Bulunamayan ya da okuma izni olmayan tablolar:"), el("ul", { class: "plain mono t-xs" }, ...missing.map((t) => el("li", {}, t))));
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
  srcResult(el("p", { class: "t-2" }, "Okunuyor…"));
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
          el("td", { class: "mono t-xs" }, inv.doc_no || inv.internal_no || ""),
          el("td", {}, inv.date ? new Date(inv.date).toLocaleDateString("tr-TR", { timeZone: "UTC" }) : ""),
          el("td", {}, (DIRECTION[inv.direction] || "") + (inv.cancelled ? " (iptal)" : "")),
          el("td", {}, inv.counterparty_title || ""),
          el("td", { class: "num" }, money(inv.totals?.net || inv.totals?.amount)),
        ),
      );
      nodes.push(el("table", { class: "grid" }, el("thead", {}, head), el("tbody", {}, ...body)));
      if (r.total > rows.length) nodes.push(el("p", { class: "t-2" }, `İlk ${rows.length} fatura gösteriliyor.`));
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
    /* kabuk ayarları okunamazsa anahtarlar olduğu gibi kalır */
  }
  syncThemeControls();
  loadOffice();
}

$("program-open").addEventListener("click", () => selectTab("program"));
$("program-back").addEventListener("click", () => selectTab("settings", true));

function setPaused(p) {
  paused = !!p;
  $("set-paused").checked = paused;
  $("paused-banner").hidden = !paused;
  if (status) {
    renderWatches(status.watches);
    renderStatusOnly();
  }
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
// Tepsi menüsü (#325) pencereyi açıp bir sekmeyi ister: "Ayrıntılar" →
// Sorunlar, son dosyalar → Geçmiş, "Ayarlar" → Ayarlar. Kurulum bitmeden
// sekmeler yok; o zaman yalnızca pencere açılır.
listen("shell://navigate", (e) => {
  if (VIEWS.includes(e.payload) && VIEWS.includes(currentView)) selectTab(e.payload, true);
});

// Görünüm: palet ve kip (tema.js; bu bilgisayarda saklanır).
function syncThemeControls() {
  const { palet, kip } = temaTercihi();
  for (const r of document.querySelectorAll("input[name=palet]")) r.checked = r.value === palet;
  for (const r of document.querySelectorAll("input[name=kip]")) r.checked = r.value === kip;
}
for (const r of document.querySelectorAll("input[name=palet], input[name=kip]")) {
  r.addEventListener("change", () => {
    const palet = document.querySelector("input[name=palet]:checked")?.value;
    const kip = document.querySelector("input[name=kip]:checked")?.value;
    temaSec({ palet, kip });
  });
}

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
      return `Klasör izleme güncelleme sırasında kısa bir süre durur, kaçan dosya olmaz.${u.notes ? ` ${u.notes}` : ""}`;
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
  disabled: ["", "Kapalı"],
  idle: ["", "Denetlenmedi"],
  checking: ["", "Denetleniyor…"],
  up_to_date: ["ok", "Güncel"],
  downloading: ["brand", "İndiriliyor…"],
  installing: ["brand", "Kuruluyor…"],
  error: ["down", "Sorun var"],
};

function showUpdate(u) {
  const [state, text] = u.state === "available" ? ["brand", `${u.available} hazır`] : updateBadge[u.state] || updateBadge.idle;
  const b = $("up-badge");
  b.dataset.state = state;
  b.textContent = text;
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
  $("cnt-settings").textContent = banner ? "•" : "";
  $("cnt-settings").setAttribute("aria-label", banner ? "güncelleme hazır" : "");
  if (banner) $("update-banner-text").textContent = `Hakan ${u.available} sürümü hazır.`;
  appVersion = u.current || appVersion;
  adv.app = u.current;
  renderAdv();
}

function showProgress({ downloaded, total }) {
  const bar = $("up-progress");
  const fill = $("up-progress-bar");
  if (total) {
    bar.max = total;
    bar.value = downloaded;
    fill.style.width = `${Math.floor((downloaded / total) * 100)}%`;
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

// --- Ofis paneli (tek program) ---------------------------------------------------
//
// Web ofis paneli kabukta ayrı bir pencerede ("office") açılır; o pencereye
// hiçbir yetenek verilmez. Bu ekran yalnızca kabuk komutlarını çağırır:
// office_status / office_open / office_reload ve indirme klasörü seçimi.

let officeInfo = null;
let newDocs = 0;
let downloads = store.get("hakan.indirilenler", []);

async function loadOffice() {
  try {
    officeInfo = await invoke("office_status");
  } catch {
    officeInfo = null;
  }
  renderOffice();
}

function renderOffice() {
  const o = officeInfo;
  $("panel-url").textContent = o?.web_url ? `Adres: ${o.web_url}` : "Panel adresi sunucu adresinden bulunur; önce oturum açın.";
  $("panel-open").textContent = o?.open ? "Ofis paneline geç" : "Ofis panelini aç";
  const dir = o?.download_dir || "";
  $("dl-dir").textContent = dir || "Seçilmedi: bilgisayarın İndirilenler klasörü kullanılıyor.";
  $("dl-dir").className = dir ? "mono t-xs" : "t-sm t-2";
  $("dl-clear").hidden = !dir;
  $("panel-dl-text").textContent = dir
    ? `Ofis panelinden indirilen Luca dosyaları “${folderName(dir)}” klasörüne kaydedilir.`
    : "Ofis panelinden indirilen Luca dosyaları bilgisayarın İndirilenler klasörüne kaydedilir. Ayarlar'dan başka bir klasör seçebilirsiniz.";
  $("panel-new").hidden = newDocs === 0;
  $("panel-new-text").textContent = newDocs === 1 ? "1 yeni belge geldi. Panelde görmek için yenileyin." : `${newDocs} yeni belge geldi. Panelde görmek için yenileyin.`;
  $("cnt-panel").textContent = newDocs ? String(newDocs) : "";
  $("panel-downloads").replaceChildren(
    ...downloads.map((d) =>
      el(
        "div",
        { class: "row t-sm", role: "listitem" },
        el("span", { class: "pill", dataset: { state: d.ok ? "ok" : "down" } }, d.ok ? "Kaydedildi" : "İndirilemedi"),
        el("span", { class: "grow file", title: d.name }, d.name),
        el("span", { class: "t-xs t-3" }, [d.folder, shortTime(d.at)].filter(Boolean).join(" · ")),
      ),
    ),
  );
}

async function officeCall(cmd) {
  show($("panel-msg"), "");
  try {
    officeInfo = await invoke(cmd);
  } catch (e) {
    show($("panel-msg"), errText(e));
  }
  renderOffice();
}

$("panel-open").addEventListener("click", () => officeCall("office_open"));
$("panel-reload").addEventListener("click", () => {
  newDocs = 0;
  officeCall("office_reload");
});
$("panel-dl-settings").addEventListener("click", () => {
  selectTab("settings");
  $("dl-pick").focus();
});
$("dl-pick").addEventListener("click", async () => {
  try {
    officeInfo = await invoke("office_pick_download_dir");
  } catch (e) {
    show($("settings-msg"), errText(e));
  }
  renderOffice();
});
$("dl-clear").addEventListener("click", async () => {
  try {
    officeInfo = await invoke("office_clear_download_dir");
  } catch (e) {
    show($("settings-msg"), errText(e));
  }
  renderOffice();
});

// Kabuk: yeni belgeler geldi ve ofis paneli açık (tepsi bildirimi de gider).
listen("office://new-documents", (e) => {
  newDocs += Number(e.payload?.count) || 0;
  renderOffice();
});
// Kabuk: ofis panelinden indirilen dosya kaydedildi (yalnızca kendi kaynağından).
listen("office://download", (e) => {
  const d = e.payload || {};
  if (!d.name) return;
  downloads = [{ name: d.name, folder: d.folder || "", ok: !!d.ok, at: new Date().toISOString() }, ...downloads].slice(0, 10);
  store.set("hakan.indirilenler", downloads);
  renderOffice();
});

// --- Sürükle-bırak yükleme (tek program) ------------------------------------------
//
// Dosya ya da klasör pencereye bırakılınca mükellef, dönem (ay) ve belge türü
// seçilir; sidecar dosyaları ofisin evrak yükleme ucuna gönderir (Luca
// dökümü ajan belgesi ucuna). Sonuç her dosya için `upload` olayıyla
// Geçmiş'e düşer.

const DOC_TYPES = ["invoice", "z_report", "expense", "bank_statement", "other"];
const looksLikeFolder = (p) => !/\.[a-z0-9]{2,5}$/i.test(p);
let dropPaths = [];

/** Son 24 ay: "Kasım 2026" → "2026-11" (en yeni önce). */
export function periodOptions(now = new Date()) {
  const out = [];
  for (let i = 0; i < 24; i++) {
    const d = new Date(now.getFullYear(), now.getMonth() - i, 1);
    const value = `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, "0")}`;
    const label = d.toLocaleDateString("tr-TR", { month: "long", year: "numeric" });
    out.push({ value, label: label.charAt(0).toLocaleUpperCase("tr") + label.slice(1) });
  }
  return out;
}

function dropSummary(paths) {
  const names = paths.map(folderName);
  if (names.length === 1) return looksLikeFolder(paths[0]) ? `“${names[0]}” klasöründeki dosyalar` : names[0];
  return `${names.length} öğe: ${names.slice(0, 3).join(", ")}${names.length > 3 ? "…" : ""}`;
}

async function openDrop(paths) {
  dropPaths = paths;
  if (!engagements.length) await loadEngagements();
  $("drop-files").textContent = dropSummary(paths);
  const last = store.get("hakan.yukleme", {});
  $("drop-taxpayer").replaceChildren(...engagements.map((e) => el("option", { value: e.id }, e.title)));
  if (engagements.some((e) => e.id === last.engagement)) $("drop-taxpayer").value = last.engagement;
  $("drop-period").replaceChildren(...periodOptions().map((o) => el("option", { value: o.value }, o.label)));
  $("drop-type").value = DOC_TYPES.includes(last.type) ? last.type : "invoice";
  $("drop-watch").hidden = !(paths.length === 1 && looksLikeFolder(paths[0]));
  show($("drop-msg"), engagements.length ? "" : "Size atanmış etkin mükellef yok.");
  $("drop-submit").disabled = !engagements.length;
  if (!$("drop-dialog").open) $("drop-dialog").showModal();
}

$("drop-cancel").addEventListener("click", () => $("drop-dialog").close());
$("drop-watch").addEventListener("click", () => {
  const path = dropPaths[0];
  $("drop-dialog").close();
  selectTab("folders");
  openAdd();
  setPicked(path);
});
$("drop-form").addEventListener("submit", async (ev) => {
  ev.preventDefault();
  const params = {
    paths: dropPaths,
    engagement_uuid: $("drop-taxpayer").value,
    period: $("drop-period").value,
    doc_type: $("drop-type").value,
  };
  if (!params.engagement_uuid) return show($("drop-msg"), "Bir mükellef seçin.");
  $("drop-submit").disabled = true;
  try {
    const r = await call("upload.drop", params);
    store.set("hakan.yukleme", { engagement: params.engagement_uuid, type: params.doc_type });
    $("drop-dialog").close();
    const n = (r.files || []).length;
    selectTab("history");
    show($("history-msg"), `${n} dosya yükleniyor. Her dosyanın sonucu aşağıda görünür.`, "info");
  } catch (e) {
    show($("drop-msg"), errText(e));
  } finally {
    $("drop-submit").disabled = false;
  }
});

/** Ayarlar → Gelişmiş: .xml dosyası bu bilgisayarda okunur, gönderilmez. */
async function inspectUBL(path) {
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
}

// Sürükle-bırak: kurulumda klasör → ilk klasör; Gelişmiş açıkken tek .xml →
// yerel denetim; klasör ekleme formu açıkken klasör → izlenecek klasör;
// diğer her durumda → belge yükleme penceresi.
listen("tauri://drag-drop", async (e) => {
  const paths = e.payload?.paths || [];
  if (!paths.length) return;
  if (currentView === "first") {
    const dir = paths.find(looksLikeFolder);
    if (dir) setFirstPicked(dir);
    return;
  }
  if (!status?.running || !VIEWS.includes(currentView)) return;
  if (currentView === "settings" && $("advanced").open && paths.length === 1 && paths[0].toLowerCase().endsWith(".xml")) {
    await inspectUBL(paths[0]);
    return;
  }
  if (currentView === "folders" && !$("add-form").hidden && paths.length === 1 && looksLikeFolder(paths[0])) {
    setPicked(paths[0]);
    return;
  }
  await openDrop(paths);
});

// --- Başlangıç --------------------------------------------------------------

window.__TAURI__.app
  ?.getVersion()
  .then((v) => {
    adv.app = v;
    appVersion = v;
    renderAdv();
  })
  .catch(() => {});
invoke("sidecar_status")
  .then((s) => s.ready && showReady(s.ready))
  .catch(() => {});
invoke("update_status").then(showUpdate).catch(() => {});
loadSettings();
renderHistory();
refresh();
setInterval(refresh, 15000);

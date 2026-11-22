#!/usr/bin/env node
// Pilot demosunun duman testi (#456): demo verisi kurulmuş, API çalışan bir
// ortamda ana akışları yalnızca okuyarak denetler. Veriyi değiştirmez; görsel
// testlerden önce de koşabilir.
//
//   make smoke                       # make demo + make backend-dev sonrası
//   node scripts/smoke.mjs           # aynısı
//
// scripts/e2e-up.sh ortam hazır olunca bunu çalıştırır (CI'daki e2e işi;
// atlamak için E2E_SKIP_SMOKE=1).
//
// Ayarlar (ortam değişkeni):
//   SMOKE_API_URL        API adresi (varsayılan http://127.0.0.1:8080)
//   SMOKE_WEB_URL        panel adresi (varsayılan http://127.0.0.1:3000);
//                        yanıt vermiyorsa atlanır, SMOKE_REQUIRE_WEB=1 ile zorunlu
//   DEMO_PASSWORD        demo hesaplarının parolası (varsayılan Demo1234!)
//   SMOKE_SA_EMAIL / SMOKE_SA_PASSWORD  platform yöneticisi; verilmezse
//                        admin@example.com / Password1 denenir, giriş yoksa atlanır
//
// Beklenen rakamlar demo hikâyesinden gelir (backend/cmd/demo-seed/ledger.go).

const API = (process.env.SMOKE_API_URL || "http://127.0.0.1:8080").replace(/\/$/, "");
const WEB = (process.env.SMOKE_WEB_URL || "http://127.0.0.1:3000").replace(/\/$/, "");
const PASSWORD = process.env.DEMO_PASSWORD || "Demo1234!";
const ORG = "demo-smmm";
const DOMAIN = "demo.hakan.local";

let failed = 0;
let passed = 0;
const ok = (msg) => {
  passed++;
  console.log(`  ✓ ${msg}`);
};
const fail = (msg) => {
  failed++;
  console.log(`  ✗ ${msg}`);
};
const check = (cond, msg, detail) => (cond ? ok(msg) : fail(detail ? `${msg} (${detail})` : msg));
const section = (name) => console.log(`\n${name}`);
const num = (v) => Number.parseFloat(v ?? "NaN");
const same = (a, b) => Math.abs(num(a) - num(b)) < 0.005;

async function request(path, { token, org, taxpayer, method = "GET", body } = {}) {
  const headers = { accept: "application/json" };
  if (token) headers.authorization = `Bearer ${token}`;
  if (org) headers["X-Organization"] = org;
  if (taxpayer) headers["X-Taxpayer"] = taxpayer;
  if (body) headers["content-type"] = "application/json";
  let res;
  try {
    res = await fetch(API + path, { method, headers, body: body ? JSON.stringify(body) : undefined });
  } catch (err) {
    return { status: 0, data: null, error: String(err.cause?.code || err.message) };
  }
  let json = null;
  try {
    json = await res.json();
  } catch {
    // not JSON (healthz)
  }
  return { status: res.status, data: json?.data ?? json, error: json?.error?.code };
}

// get fails the check when the call is not 200 and returns data (or null).
async function get(label, path, opts) {
  const r = await request(path, opts);
  if (r.status !== 200) {
    fail(`${label}: HTTP ${r.status}${r.error ? ` ${r.error}` : ""}`);
    return null;
  }
  return r.data;
}

async function login(email, password) {
  const r = await request("/v1/auth/login", { method: "POST", body: { email, password } });
  return r.status === 200 ? r.data?.access_token : null;
}

async function main() {
  console.log(`Duman testi: API ${API}`);

  section("Sağlık");
  const health = await request("/healthz");
  if (health.status === 0) {
    fail(`API yanıt vermiyor (${health.error}); önce make demo ve make backend-dev`);
    return;
  }
  check(health.status === 200, "/healthz", `HTTP ${health.status}`);
  const ready = await request("/readyz");
  check(ready.status === 200 && ready.data?.status === "ready", "/readyz hazır", JSON.stringify(ready.data?.checks ?? ready.status));

  section("Girişler");
  const tokens = {};
  for (const who of ["sahip", "musavir", "stajyer", "mukellef"]) {
    tokens[who] = await login(`${who}@${DOMAIN}`, PASSWORD);
    check(Boolean(tokens[who]), `${who}@${DOMAIN} giriş yapar`);
  }
  const saEmail = process.env.SMOKE_SA_EMAIL || "admin@example.com";
  const saToken = await login(saEmail, process.env.SMOKE_SA_PASSWORD || "Password1");
  if (saToken) {
    const orgs = await get("platform organizasyonları", "/v1/platform/organizations", { token: saToken });
    const items = orgs?.items ?? orgs ?? [];
    check(Array.isArray(items) && items.some((o) => o.slug === ORG), `platform yöneticisi (${saEmail}) demo ofisini görür`);
  } else if (process.env.SMOKE_SA_EMAIL) {
    fail(`platform yöneticisi ${saEmail} giriş yapamadı`);
  } else {
    console.log(`  - platform yöneticisi (${saEmail}) yok, atlandı`);
  }
  if (!tokens.musavir) return;

  section("Ofis paneli (müşavir)");
  const office = { token: tokens.musavir, org: ORG };
  const engs = await get("mükellef listesi", "/v1/tenant/engagements", office);
  const items = engs?.items ?? [];
  const atlas = items.find((e) => e.taxpayer_title?.startsWith("Atlas Gıda"));
  check(Boolean(atlas) && items.some((e) => e.taxpayer_title === "Selin Yılmaz"), "Atlas Gıda ve Selin Yılmaz listede", `${items.length} mükellef`);
  if (!atlas) return;
  const E = `/v1/tenant/engagements/${atlas.uuid}`;

  const runs = (await get("bordro dönemleri", `${E}/payroll/runs`, office))?.items ?? [];
  const sep = runs.find((r) => r.period === "2026-09");
  check(runs.length === 2 && same(sep?.totals?.net_total, "124741.51"), "bordro: Eylül ve Ekim, Eylül neti 124.741,51", `${runs.length} dönem, net ${sep?.totals?.net_total}`);
  const prem = (await get("SGK primleri", `${E}/payroll/sgk-premiums`, office))?.items ?? [];
  const sepPrem = prem.find((p) => p.period === "2026-09");
  check(sepPrem?.paid_at?.startsWith("2026-10-30") && same(sepPrem?.paid_amount, "50701.79"), "SGK: Eylül primi 30.10'da 50.701,79 ödendi", `${sepPrem?.paid_at} ${sepPrem?.paid_amount}`);

  // Garanti hesabı muhasebe farkı örneğidir (#780 C); bu kontrol Ziraat ekstresine bakar.
  // Gelen kutusunun bu ayki eşleşmemiş havalesi (#780 b) Ekim ekstresinden sayılmaz.
  const txns = ((await get("banka hareketleri", `${E}/bank-transactions?limit=100`, office))?.items ?? []).filter(
    (t) => !t.cash_box_name?.startsWith("Garanti") && !t.description?.startsWith("HAVALE GELEN SERDAR AKIN"),
  );
  const pays = {
    "MAAS ODEMESI EYLUL PERSONEL": "124741.51",
    "SGK PRIM ODEMESI 2026/09": "50701.79",
    "GIB VERGI ODEMESI KDV 2026/09": "10150.00",
    "GIB VERGI ODEMESI MUHSGK 2026/09": "7863.16",
  };
  const bad = Object.entries(pays).filter(([d, a]) => !txns.some((t) => t.description === d && same(t.amount, a)));
  check(txns.length === 9 && bad.length === 0, "banka: Ekim ekstresi Eylül maaş, SGK, KDV ve MUHSGK'yı öder", `${txns.length} hareket, eksik: ${bad.map(([d]) => d).join(", ")}`);
  const queue = (await get("banka eşleme kuyruğu", `${E}/bank-postings?period=2026-10&status=suggested`, office))?.items ?? [];
  check(queue.length >= 1, "banka eşleme kuyruğunda bekleyen hareket var", `${queue.length}`);

  const exp = (await get("masraflar", `${E}/expenses?period=2026-10&limit=100`, office))?.items ?? [];
  const z = (await get("Z raporları", `${E}/retail-sales?period=2026-10&limit=100`, office))?.items ?? [];
  check(exp.length > 0 && z.length > 0, "masraf ve Z raporları yüklü", `${exp.length} masraf, ${z.length} Z`);
  const warns = (await get("uyarılar", `${E}/warnings?limit=200`, office))?.items ?? [];
  check(warns.some((w) => w.code === "Z_NUMBER_GAP") && warns.some((w) => w.code === "Z_DAY_GAP"), "Z süreklilik uyarıları açık");

  const tbs = (await get("mizanlar", `${E}/account-plan/trial-balances`, office))?.items ?? [];
  check(["2026-09", "2026-10"].every((p) => tbs.some((t) => t.period === p)), "mizan: Eylül ve Ekim yüklü");

  const rec = await get("Ekim mutabakatı", `${E}/reconciliation/runs/latest?period=2026-10`, office);
  const open = (rec?.items ?? []).filter((i) => i.result !== "matched").map((i) => i.main_code).sort();
  check(open.join(",") === "100,108,120,391,600", "mutabakat: yalnızca Z taslakları (100, 108, 391, 600) ve elle fiş (120) açık", open.join(","));

  const returns = (await get("beyannameler", `${E}/tax-returns`, office))?.items ?? [];
  const kdv = returns.find((r) => r.kind === "kdv1" && r.period === "2026-09");
  const muh = returns.find((r) => r.kind === "muhsgk" && r.period === "2026-09");
  check(kdv?.status === "filed" && same(kdv?.payable, "10150"), "beyanname: Eylül KDV1 verildi, ödenecek 10.150,00", `${kdv?.status} ${kdv?.payable}`);
  check(muh?.status === "filed" && same(muh?.payable, "7863.16"), "beyanname: Eylül MUHSGK verildi, 7.863,16", `${muh?.status} ${muh?.payable}`);
  check(returns.some((r) => r.kind === "kdv1" && r.period === "2026-10" && r.status === "draft"), "beyanname: Ekim KDV1 taslak");

  const drafts = (await get("yevmiye taslakları", `${E}/journal-drafts?limit=100`, office))?.items ?? [];
  const exports = (await get("Luca / Zirve çıktıları", `${E}/journal-exports`, office))?.items ?? [];
  check(drafts.some((d) => d.status === "draft") && drafts.some((d) => d.status === "exported"), "yevmiye: aktarılmış ve bekleyen fişler", `${drafts.length} fiş`);
  check(exports.some((x) => x.driver === "luca") && exports.some((x) => x.driver === "zirve"), "yevmiye: Luca ve Zirve çıktısı var");

  const et = (await get("Easytrade bağlantısı", `${E}/easytrade-connections`, office))?.items ?? [];
  check(et.some((c) => c.status === "active"), "Easytrade: demo bağlantısı etkin");
  const report = await get("Ekim raporu", `${E}/report?period=2026-10`, office);
  check(Boolean(report?.totals), "rapor: Ekim 2026 özeti");

  section("Yetki (stajyer)");
  if (tokens.stajyer) {
    const st = { token: tokens.stajyer, org: ORG };
    const list = (await get("stajyerin mükellefleri", "/v1/tenant/engagements", st))?.items ?? [];
    check(list.length === 1 && list[0].taxpayer_title === "Selin Yılmaz", "stajyer yalnızca atandığı Selin Yılmaz'ı görür", list.map((e) => e.taxpayer_title).join(", "));
    const denied = await request(`${E}/payroll/runs`, st);
    check(denied.status === 403 || denied.status === 404, "stajyer Atlas Gıda'nın bordrosuna erişemez", `HTTP ${denied.status}`);
  }

  section("Mükellef paneli");
  if (tokens.mukellef) {
    const me = await get("mükellef hesabı", "/v1/auth/me", { token: tokens.mukellef });
    const tp = (me?.taxpayers ?? []).find((t) => t.title?.startsWith("Atlas Gıda"));
    check(Boolean(tp), "mükellef kullanıcısı Atlas Gıda'nın yetkilisi");
    if (tp) {
      const m = { token: tokens.mukellef, taxpayer: tp.uuid };
      const ctx = await get("mükellef bağlamı", "/v1/taxpayer/context", m);
      check(ctx?.taxpayer?.tax_id === "9000000017", "mükellef bağlamı Atlas Gıda");
      const docs = (await get("bordro belgeleri", "/v1/taxpayer/payroll/documents", m))?.items ?? [];
      check(docs.length === 4, "mükellef 4 bordro / SGK belgesini görür", `${docs.length}`);
      const rep = await get("mükellef raporu", "/v1/taxpayer/report?period=2026-10", m);
      check(Boolean(rep?.totals), "mükellef Ekim raporunu görür");
      await get("mükellef faturaları", "/v1/taxpayer/invoices", m).then((d) => d && ok("mükellef faturalar sayfası açılır"));
    }
  }

  section("Panel");
  let web;
  try {
    web = await fetch(`${WEB}/login`, { redirect: "manual" });
  } catch {
    web = null;
  }
  if (web) {
    check(web.status === 200, `${WEB}/login açılır`, `HTTP ${web.status}`);
  } else if (process.env.SMOKE_REQUIRE_WEB === "1") {
    fail(`${WEB} yanıt vermiyor`);
  } else {
    console.log(`  - ${WEB} yanıt vermiyor, atlandı (make frontend-dev)`);
  }
}

await main();
console.log(`\n${passed} kontrol geçti, ${failed} kontrol düştü.`);
process.exit(failed === 0 ? 0 : 1);

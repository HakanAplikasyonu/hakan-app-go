// Kılavuz ve Chrome Web Store görüntüleri (#780). Uzantıyı sahte sunucu
// adresi gömülü olarak derler (açılır pencere mağazadaki gibi yalnızca kod +
// Eşleştir gösterir), Chromium'a yükler; WhatsApp Web yerine temsilî bir
// sohbet sayfası (tests/fixtures/chat.html yapısı, WhatsApp logosu ya da
// renkleri yok), API yerine sahte yanıtlar sunar. Bütün adlar ve veriler
// uydurmadır; gerçek sohbet, kişi ya da sunucu kullanılmaz.
//
// Çıktılar:
//   frontend/public/yardim/img/whatsapp-*.png        kılavuz (docs/kullanim/whatsapp-uzantisi.md)
//   extensions/whatsapp/store-gorseller/             mağaza (commit'lenmez):
//     ekran-1..4-1280x800.png, tanitim-440x280.png, simge-128.png
//
// Playwright depoda e2e/ altında kurulu (CI: extension-whatsapp-gorseller işi):
//   npm ci && NODE_PATH=../../e2e/node_modules node scripts/screenshots.mjs
import { execFileSync } from "node:child_process";
import { createRequire } from "node:module";
import { mkdirSync, mkdtempSync, readFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";

const require = createRequire(import.meta.url);
const { chromium } = require("playwright");

const root = join(dirname(fileURLToPath(import.meta.url)), "..");
const guideOut = join(root, "../../frontend/public/yardim/img");
const storeOut = join(root, "store-gorseller");
mkdirSync(storeOut, { recursive: true });
const ext = join(root, "dist");
const API = "https://api.ofis.example";

// Sunucu adresi gömülü derleme (mağaza paketi gibi).
execFileSync("node", [join(root, "scripts/build.mjs")], { stdio: "inherit", env: { ...process.env, HAKAN_API_URL: API } });

const icon128 = "data:image/png;base64," + readFileSync(join(root, "static/icons/128.png")).toString("base64");

// Kılavuzda test metinleri yerine doğal mesajlar görünsün.
const chat = readFileSync(join(root, "tests/fixtures/chat.html"), "utf8")
  .replace("GİZLİ MESAJ METNİ: kira ödemesi yarın", "Merhaba, ekim fişlerini gönderiyorum")
  .replace("Akaryakıt fişi GİZLİ AÇIKLAMA", "Akaryakıt fişi");

const atlas = { engagement_uuid: "0b6f6c1e-6a52-4c39-9a47-3a7a3b0c0001", taxpayer_uuid: "0b6f6c1e-6a52-4c39-9a47-3a7a3b0c0002", title: "Atlas Gıda Ticaret A.Ş.", tax_id: "9000000017" };
const selin = { engagement_uuid: "0b6f6c1e-6a52-4c39-9a47-3a7a3b0c0003", taxpayer_uuid: "0b6f6c1e-6a52-4c39-9a47-3a7a3b0c0004", title: "Selin Yılmaz", tax_id: "10000000146" };
// Hizmet çalışanı sunucu izni olmadan da okuyabilsin diye CORS başlıkları.
const cors = { "access-control-allow-origin": "*", "access-control-allow-headers": "authorization, content-type, x-organization", "access-control-allow-methods": "GET, POST, PUT, OPTIONS" };
const env = (data, status = 200) => ({ status, headers: cors, contentType: "application/json", body: JSON.stringify({ success: true, data, meta: {} }) });

// Temsilî mesajlaşma sayfası: sol sohbet listesi şematik, sağda fixture'ın
// yapısı. Nötr renkler; WhatsApp logosu ya da marka öğesi kullanılmaz.
const sidebar = ["Okan Atlas (Atlas Gıda)", "Selin Yılmaz", "Demir İnşaat Muhasebe", "Kaya Kırtasiye", "Ofis ekibi"]
  .map((n, i) => `<div class="c${i === 0 ? " on" : ""}"><i></i><span>${n}</span></div>`)
  .join("");
const page = `<!doctype html><html lang="tr"><head><meta charset="utf-8"><style>
body{margin:0;font:14px/1.4 system-ui,-apple-system,"Segoe UI",sans-serif;background:#e9edf2;color:#1d2733;display:flex;height:100vh;overflow:hidden}
.side{width:320px;flex:none;background:#fff;border-right:1px solid #d5dbe3}
.side h2{margin:0;padding:18px 16px;font-size:17px;background:#f4f6f9;border-bottom:1px solid #d5dbe3}
.side .c{display:flex;align-items:center;gap:12px;padding:12px 16px;border-bottom:1px solid #eef1f5}
.side .c.on{background:#e8edf6}
.side i{width:40px;height:40px;border-radius:50%;background:#b9c4d3;flex:none}
#app{flex:1;overflow:auto}
#main{min-height:100vh;background:#e9edf2}
#main header{background:#f4f6f9;padding:16px;font-weight:600;border-bottom:1px solid #d5dbe3}
[role=row]{display:flex;padding:4px 24px}
[data-id^="true_"]{margin-left:auto}
.message-in,.message-out{background:#fff;border-radius:8px;padding:6px;max-width:360px;box-shadow:0 1px 1px rgba(20,30,45,.12)}
.message-out{background:#dfe7f7}
img{display:block;border-radius:6px;max-width:330px;height:auto}
[data-testid=document-thumb]{display:inline-block;width:28px;height:34px;background:#d6453d;border-radius:3px;vertical-align:middle;margin-right:8px}
[data-testid=document-name]{vertical-align:middle}
button[aria-label="İndir"]{margin-left:8px;border:0;background:#eef1f5;border-radius:50%;width:28px;height:28px}
[data-testid=sticker-message]{background:transparent;box-shadow:none}
</style></head><body><nav class="side"><h2>Sohbetler</h2>${sidebar}</nav>${chat}<script>
// Görselleri gerçek blob adresiyle çiz (indirilmiş görsel gibi).
for (const img of document.querySelectorAll('img[src^="blob:"]')) {
  const c = document.createElement('canvas'); c.width = 330; c.height = 440;
  const g = c.getContext('2d'); g.fillStyle = '#fafafa'; g.fillRect(0,0,330,440);
  g.fillStyle = '#222'; g.font = 'bold 18px monospace'; g.fillText('ATLAS AKARYAKIT', 70, 50);
  g.font = '14px monospace';
  ['14.10.2026  09:41','VKN 1234567890','KURŞUNSUZ 95   41,20 L','TOPLAM      ₺1.250,00','KDV %20       ₺208,33','FİŞ NO 0042'].forEach((l,i)=>g.fillText(l, 50, 110+i*34));
  c.toBlob(b => { img.src = URL.createObjectURL(b); });
}
// Henüz indirilmemiş görsel: bulanık önizleme.
for (const img of document.querySelectorAll('img[src^="data:image"]')) {
  const c = document.createElement('canvas'); c.width = 33; c.height = 44;
  const g = c.getContext('2d'); const gr = g.createLinearGradient(0,0,33,44);
  gr.addColorStop(0,'#c9c3b8'); gr.addColorStop(1,'#8f9aa3'); g.fillStyle = gr; g.fillRect(0,0,33,44);
  img.src = c.toDataURL('image/jpeg'); img.style.width = '330px'; img.style.height = '440px'; img.style.filter = 'blur(6px)';
}
</script></body></html>`;

const profile = mkdtempSync(join(tmpdir(), "hakan-wa-shots-"));
const ctx = await chromium.launchPersistentContext(profile, {
  channel: "chromium",
  headless: true,
  viewport: { width: 1280, height: 800 },
  deviceScaleFactor: 2,
  locale: "tr-TR",
  timezoneId: "Europe/Istanbul",
  args: [`--disable-extensions-except=${ext}`, `--load-extension=${ext}`],
});
await ctx.route("https://web.whatsapp.com/**", (r) => r.fulfill({ status: 200, contentType: "text/html; charset=utf-8", body: page }));
let linked = false;
await ctx.route(`${API}/**`, (r) => {
  const url = new URL(r.request().url());
  const p = url.pathname;
  if (r.request().method() === "OPTIONS") return r.fulfill({ status: 204, headers: cors });
  if (p === "/v1/auth/whatsapp/pair") {
    return r.fulfill(env({ access_token: "a", refresh_token: "r", expires_in: 900, token_type: "Bearer", refresh_expires_at: "2026-12-24T00:00:00Z", user: { uuid: "u", email: "mert@ofis.example", name: "Mert Demir" }, organization: { uuid: "o", slug: "demo-smmm", name: "Demir Mali Müşavirlik" } }));
  }
  if (p === "/v1/auth/me") return r.fulfill(env({ uuid: "u" }));
  if (p === "/v1/tenant/whatsapp/chat-lookup") {
    return r.fulfill(env({ chat_key: "900000000000", link: linked ? { uuid: "l", chat_key: "900000000000", label: "Okan Atlas", taxpayer: atlas } : null, suggestions: [atlas] }));
  }
  if (p === "/v1/tenant/inbox") {
    return r.fulfill(env({ items: [], counts: { invoice: 6, bank_transaction: 4, expense: 3, retail_sale: 1, foreign_invoice: 0, late_document: 0, total: 14 } }));
  }
  if (p === "/v1/tenant/vat-forecast/watch-events") {
    return r.fulfill(env({ items: [
      { uuid: "v1", period: "2026-11", event: "vat.threshold_exceeded", engagement_uuid: atlas.engagement_uuid, taxpayer_title: "Atlas Gıda Ticaret A.Ş.", created_at: "2026-11-26T09:00:00Z" },
      { uuid: "v2", period: "2026-10", event: "vat.report_ready", engagement_uuid: selin.engagement_uuid, taxpayer_title: "Selin Yılmaz", created_at: "2026-11-24T09:00:00Z" },
    ] }));
  }
  if (p === "/v1/tenant/clients") {
    if (url.searchParams.get("filter") === "tasks") {
      return r.fulfill(env({ items: [{ ...atlas, open_tasks: 2 }, { ...selin, open_tasks: 1 }], limit: 5, next_cursor: null }));
    }
    return r.fulfill(env({ items: [atlas, selin], limit: 20, next_cursor: null }));
  }
  if (p.endsWith("/whatsapp/chat-link")) {
    linked = true;
    return r.fulfill(env({ chat_key: "900000000000", link: { uuid: "l", chat_key: "900000000000", label: "Okan Atlas", taxpayer: atlas }, suggestions: [] }));
  }
  if (p.endsWith("/document-tasks")) return r.fulfill(env({ items: [], total: 0, limit: 20, offset: 0 }));
  if (p.endsWith("/whatsapp/captures")) {
    return r.fulfill(env({ uuid: "c", target: "expense", file_name: "fis.png", content_type: "image/png", size_bytes: 1000, taxpayer: atlas, created_by_name: "Mert", receipt_uuid: "r", receipt_status: "draft", task_uuid: null, task_title: null, has_file: false, created_at: "2026-11-17T09:00:00Z", replayed: false }, 201));
  }
  return r.fulfill({ status: 404, headers: cors, contentType: "application/json", body: "{}" });
});

let [sw] = ctx.serviceWorkers();
if (!sw) sw = await ctx.waitForEvent("serviceworker");
const id = new URL(sw.url()).host;
// Sunucu iznini önceden ver (gerçekte açılır pencere izin ister).
await sw.evaluate((api) => chrome.permissions.request({ origins: [api + "/*"] }).catch(() => false), API);

// 1) Eşleştirme penceresi: yalnızca kod + Eşleştir (adres pakete gömülü).
const pop = await ctx.newPage();
await pop.setViewportSize({ width: 340, height: 420 });
await pop.goto(`chrome-extension://${id}/popup.html`);
await pop.fill("#code", "K7QM-3XPA");
const popPair = await pop.screenshot({ path: join(guideOut, "whatsapp-eslestirme.png"), fullPage: true });
// Eşleştir (izin isteği başsız tarayıcıda otomatik kabul edilmez: arka plana doğrudan gönder).
await pop.evaluate((api) => chrome.runtime.sendMessage({ type: "pair", apiBase: api, code: "K7QM-3XPA", deviceName: "Ofis · Chrome" }), API);
await pop.setViewportSize({ width: 340, height: 600 });
await pop.reload();
await pop.waitForSelector("#open-wa:visible");
await pop.waitForFunction(() => !document.getElementById("inbox")?.hidden);
const popPaired = await pop.screenshot({ path: join(guideOut, "whatsapp-bagli.png"), fullPage: true });
await pop.close();

// 2) Sohbette düğmeler.
const wa = await ctx.newPage();
await wa.goto("https://web.whatsapp.com/");
await wa.waitForFunction(() => document.querySelectorAll(".hakan-wa-bar").length >= 3);
await wa.waitForTimeout(400);
await wa.screenshot({ path: join(guideOut, "whatsapp-dugmeler.png"), clip: { x: 320, y: 0, width: 760, height: 640 } });
const shotButtons = await wa.screenshot();

// 3) İlk kayıtta mükellef seçimi.
await wa.evaluate(() => document.querySelector(".hakan-wa-bar").shadowRoot.querySelector('button[data-target="expense"]').click());
await wa.waitForFunction(() => document.querySelector(".hakan-wa-picker"));
await wa.waitForTimeout(300);
const shotPicker = await wa.screenshot({ path: join(guideOut, "whatsapp-mukellef-secimi.png") });

// 4) Kaydedildi bildirimi.
await wa.evaluate(() => document.querySelector(".hakan-wa-picker").shadowRoot.querySelector("li button").click());
await wa.waitForFunction(() => document.querySelector(".hakan-wa-toast")?.shadowRoot?.textContent?.includes("taslağı"));
await wa.screenshot({ path: join(guideOut, "whatsapp-kaydedildi.png"), clip: { x: 320, y: 400, width: 760, height: 400 } });
const shotSaved = await wa.screenshot();
await wa.close();

// Mağaza görüntüleri: 1280×800 çerçeve (solda başlık, sağda ekran).
const img = (buf) => "data:image/png;base64," + buf.toString("base64");
const frameCss = `*{box-sizing:border-box}body{margin:0;width:1280px;height:800px;display:flex;font-family:system-ui,-apple-system,"Segoe UI",sans-serif;background:#f3f5f9;overflow:hidden}
.cap{width:380px;flex:none;background:linear-gradient(160deg,#2448FF,#1631b8);color:#fff;padding:56px 40px;display:flex;flex-direction:column;gap:20px}
.brand{display:flex;align-items:center;gap:12px;font-weight:600;font-size:16px;opacity:.95}.brand img{width:36px;height:36px}
h1{margin:24px 0 0;font-size:34px;line-height:1.2}p{margin:0;font-size:18px;line-height:1.5;opacity:.92}
.note{margin-top:auto;font-size:13px;opacity:.75}
.shot{flex:1;display:flex;align-items:center;justify-content:center;gap:28px;padding:32px}
.shot img{border-radius:12px;box-shadow:0 12px 40px rgba(20,30,60,.22);background:#fff}`;
const frame = (title, text, shots, note = "Temsilî görünüm; adlar ve tutarlar uydurmadır.") =>
  `<!doctype html><html lang="tr"><head><meta charset="utf-8"><style>${frameCss}</style></head><body>
<div class="cap"><div class="brand"><img src="${icon128}" alt="">Easytrade Müşavirim Ofis</div><h1>${title}</h1><p>${text}</p><div class="note">${note}</div></div>
<div class="shot">${shots}</div></body></html>`;

// deviceScaleFactor 2 olan bağlamdan ayrı: tam 1280×800 piksel için 1x tarayıcı.
const flat = await chromium.launch({ channel: "chromium", headless: true });
const flatPage = await flat.newPage({ viewport: { width: 1280, height: 800 }, deviceScaleFactor: 1, locale: "tr-TR" });
const renderFlat = async (html, file, size = { width: 1280, height: 800 }, transparent = false) => {
  await flatPage.setViewportSize(size);
  await flatPage.setContent(html, { waitUntil: "load" });
  await flatPage.screenshot({ path: join(storeOut, file), omitBackground: transparent });
};

await renderFlat(
  frame("Fişi WhatsApp'tan tek tıkla kaydedin", "Mükelleften gelen görselin ya da PDF'in altında Gider, Gelir, Belge ve Yabancı fatura düğmeleri çıkar.", `<img src="${img(shotButtons)}" width="840" alt="">`),
  "ekran-1-1280x800.png",
);
await renderFlat(
  frame("Mükellefi bir kez seçin", "Uzantı sohbeti mükellefe eşler; telefonu eşleşen mükellef üstte önerilir. Sonraki kayıtlarda sorulmaz.", `<img src="${img(shotPicker)}" width="840" alt="">`),
  "ekran-2-1280x800.png",
);
await renderFlat(
  frame("Taslak hazır, ofiste onaylanır", "Fiş okunur ve onay bekleyen bir gider taslağı oluşur. Belge, mükellefin belge isteğine eklenir.", `<img src="${img(shotSaved)}" width="840" alt="">`),
  "ekran-3-1280x800.png",
);
await renderFlat(
  frame("Panel koduyla eşleştirin", "Parola yazılmaz: panelden alınan tek kullanımlık kodla bağlanır. Bağlanınca Gelen kutusu ve KDV uyarıları da görünür.", `<img src="${img(popPair)}" width="300" alt=""><img src="${img(popPaired)}" width="300" alt="">`),
  "ekran-4-1280x800.png",
);

// Küçük tanıtım görseli 440×280.
await renderFlat(
  `<!doctype html><html lang="tr"><head><meta charset="utf-8"><style>
body{margin:0;width:440px;height:280px;overflow:hidden;font-family:system-ui,-apple-system,"Segoe UI",sans-serif;background:linear-gradient(150deg,#2448FF,#1631b8);color:#fff;display:flex;flex-direction:column;justify-content:center;padding:0 32px;box-sizing:border-box;gap:12px}
.b{display:flex;align-items:center;gap:12px;font-weight:600;font-size:17px}.b img{width:48px;height:48px}
h1{margin:0;font-size:27px;line-height:1.2}
.t{display:flex;gap:8px;flex-wrap:wrap}.t span{background:rgba(255,255,255,.18);border-radius:999px;padding:4px 12px;font-size:13px;font-weight:600}
</style></head><body><div class="b"><img src="${icon128}" alt="">Easytrade Müşavirim Ofis</div>
<h1>WhatsApp'tan gelen fişi tek tıkla kaydedin</h1><div class="t"><span>Gider</span><span>Gelir</span><span>Belge</span><span>Yabancı fatura</span></div></body></html>`,
  "tanitim-440x280.png",
  { width: 440, height: 280 },
);

// Mağaza simgesi 128×128: 96×96 çizim + 16 px saydam kenar (Chrome Web
// Store önerisi). Mevcut simgenin (static/icons/128.png) küçültülmüşü.
await renderFlat(
  `<!doctype html><html><head><style>html,body{margin:0;background:transparent}img{display:block;width:96px;height:96px;margin:16px}</style></head><body><img src="${icon128}" alt=""></body></html>`,
  "simge-128.png",
  { width: 128, height: 128 },
  true,
);

await flat.close();
await ctx.close();
console.log("görüntüler: frontend/public/yardim/img/whatsapp-*.png ve extensions/whatsapp/store-gorseller/");

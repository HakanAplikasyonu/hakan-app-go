// Kılavuz görüntüleri (docs/kullanim/whatsapp-uzantisi.md): derlenmiş
// uzantıyı Chromium'a yükler, WhatsApp Web yerine tests/fixtures/chat.html'i
// ve API yerine sahte yanıtları sunar; görüntüleri
// frontend/public/yardim/img/ altına yazar. Gerçek sohbet ya da sunucu
// kullanılmaz. Önce `npm run build`; Playwright depoda e2e/ altında kurulu:
//   NODE_PATH=../../e2e/node_modules node scripts/screenshots.mjs
import { createRequire } from "node:module";
import { mkdtempSync, readFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";

const require = createRequire(import.meta.url);
const { chromium } = require("playwright");

const root = join(dirname(fileURLToPath(import.meta.url)), "..");
const out = join(root, "../../frontend/public/yardim/img");
const ext = join(root, "dist");
const API = "https://api.ofis.example";
// Kılavuzda test metinleri yerine doğal mesajlar görünsün.
const chat = readFileSync(join(root, "tests/fixtures/chat.html"), "utf8")
  .replace("GİZLİ MESAJ METNİ: kira ödemesi yarın", "Merhaba, ekim fişlerini gönderiyorum")
  .replace("Akaryakıt fişi GİZLİ AÇIKLAMA", "Akaryakıt fişi");

const atlas = { engagement_uuid: "0b6f6c1e-6a52-4c39-9a47-3a7a3b0c0001", taxpayer_uuid: "0b6f6c1e-6a52-4c39-9a47-3a7a3b0c0002", title: "Atlas Gıda Ticaret A.Ş.", tax_id: "9000000017" };
const selin = { engagement_uuid: "0b6f6c1e-6a52-4c39-9a47-3a7a3b0c0003", taxpayer_uuid: "0b6f6c1e-6a52-4c39-9a47-3a7a3b0c0004", title: "Selin Yılmaz", tax_id: "10000000146" };
const env = (data, status = 200) => ({ status, contentType: "application/json", body: JSON.stringify({ success: true, data, meta: {} }) });

// WhatsApp Web'e benzer sade görünüm; yapı fixture'dan gelir.
const page = `<!doctype html><html lang="tr"><head><meta charset="utf-8"><style>
body{margin:0;font:14px/1.4 system-ui,-apple-system,"Segoe UI",sans-serif;background:#efeae2;color:#111b21}
#main{max-width:720px;margin:0 auto;min-height:100vh;background:#efeae2}
#main header{background:#f0f2f5;padding:12px 16px;font-weight:600;border-bottom:1px solid #d1d7db}
[role=row]{display:flex;padding:4px 16px}
[data-id^="true_"]{margin-left:auto}
.message-in,.message-out{background:#fff;border-radius:8px;padding:6px;max-width:360px;box-shadow:0 1px .5px rgba(11,20,26,.13)}
.message-out{background:#d9fdd3}
img{display:block;border-radius:6px;max-width:330px;height:auto}
[data-testid=document-thumb]{display:inline-block;width:28px;height:34px;background:#e53935;border-radius:3px;vertical-align:middle;margin-right:8px}
[data-testid=document-name]{vertical-align:middle}
button[aria-label="İndir"]{margin-left:8px;border:0;background:#f0f2f5;border-radius:50%;width:28px;height:28px}
[data-testid=sticker-message]{background:transparent;box-shadow:none}
</style></head><body>${chat}<script>
// Görselleri gerçek blob adresiyle çiz (WhatsApp'ta indirilmiş görsel gibi).
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
  viewport: { width: 1100, height: 760 },
  deviceScaleFactor: 2,
  args: [`--disable-extensions-except=${ext}`, `--load-extension=${ext}`],
});
await ctx.route("https://web.whatsapp.com/**", (r) => r.fulfill({ status: 200, contentType: "text/html; charset=utf-8", body: page }));
let linked = false;
await ctx.route(`${API}/**`, (r) => {
  const url = new URL(r.request().url());
  const p = url.pathname;
  if (p === "/v1/auth/whatsapp/pair") {
    return r.fulfill(env({ access_token: "a", refresh_token: "r", expires_in: 900, token_type: "Bearer", refresh_expires_at: "2026-11-24T00:00:00Z", user: { uuid: "u", email: "sahip@demo.hakan.local", name: "Mert Demir" }, organization: { uuid: "o", slug: "demo-smmm", name: "Demir Mali Müşavirlik" } }));
  }
  if (p === "/v1/tenant/whatsapp/chat-lookup") {
    return r.fulfill(env({ chat_key: "905321234567", link: linked ? { uuid: "l", chat_key: "905321234567", label: "Okan Atlas", taxpayer: atlas } : null, suggestions: [atlas] }));
  }
  if (p === "/v1/tenant/clients") return r.fulfill(env({ items: [atlas, selin], limit: 20, next_cursor: null }));
  if (p.endsWith("/whatsapp/chat-link")) {
    linked = true;
    return r.fulfill(env({ chat_key: "905321234567", link: { uuid: "l", chat_key: "905321234567", label: "Okan Atlas", taxpayer: atlas }, suggestions: [] }));
  }
  if (p.endsWith("/document-tasks")) return r.fulfill(env({ items: [], total: 0, limit: 20, offset: 0 }));
  if (p.endsWith("/whatsapp/captures")) {
    return r.fulfill(env({ uuid: "c", target: "expense", file_name: "fis.png", content_type: "image/png", size_bytes: 1000, taxpayer: atlas, created_by_name: "Mert", receipt_uuid: "r", receipt_status: "draft", task_uuid: null, task_title: null, has_file: false, created_at: "2026-11-17T09:00:00Z", replayed: false }, 201));
  }
  return r.fulfill({ status: 404, body: "{}" });
});

let [sw] = ctx.serviceWorkers();
if (!sw) sw = await ctx.waitForEvent("serviceworker");
const id = new URL(sw.url()).host;
// Sunucu iznini önceden ver (gerçekte açılır pencere izin ister).
await sw.evaluate((api) => chrome.permissions.request({ origins: [api + "/*"] }).catch(() => false), API);

// 1) Eşleştirme penceresi.
const pop = await ctx.newPage();
await pop.setViewportSize({ width: 340, height: 520 });
await pop.goto(`chrome-extension://${id}/popup.html`);
await pop.fill("#base", API);
await pop.fill("#code", "K7QM-3XPA");
await pop.fill("#device", "Ofis · Chrome");
await pop.screenshot({ path: join(out, "whatsapp-eslestirme.png") });
// Eşleştir (izin isteği başsız tarayıcıda otomatik kabul edilmez: arka plana doğrudan gönder).
await pop.evaluate((api) => chrome.runtime.sendMessage({ type: "pair", apiBase: api, code: "K7QM-3XPA", deviceName: "Ofis · Chrome" }), API);
await pop.reload();
await pop.screenshot({ path: join(out, "whatsapp-bagli.png") });
await pop.close();

// 2) Sohbette düğmeler.
const wa = await ctx.newPage();
await wa.goto("https://web.whatsapp.com/");
await wa.waitForFunction(() => document.querySelectorAll(".hakan-wa-bar").length >= 3);
await wa.waitForTimeout(400);
await wa.screenshot({ path: join(out, "whatsapp-dugmeler.png"), clip: { x: 190, y: 0, width: 720, height: 640 } });

// 3) İlk kayıtta mükellef seçimi.
await wa.evaluate(() => document.querySelector(".hakan-wa-bar").shadowRoot.querySelector('button[data-target="expense"]').click());
await wa.waitForFunction(() => document.querySelector(".hakan-wa-picker"));
await wa.waitForTimeout(300);
await wa.screenshot({ path: join(out, "whatsapp-mukellef-secimi.png") });

// 4) Kaydedildi bildirimi.
await wa.evaluate(() => document.querySelector(".hakan-wa-picker").shadowRoot.querySelector("li button").click());
await wa.waitForFunction(() => document.querySelector(".hakan-wa-toast")?.shadowRoot?.textContent?.includes("taslağı"));
await wa.screenshot({ path: join(out, "whatsapp-kaydedildi.png"), clip: { x: 190, y: 360, width: 720, height: 400 } });
await ctx.close();
console.log("görüntüler: frontend/public/yardim/img/whatsapp-*.png");

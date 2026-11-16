// Masaüstü arayüz testi (#206): ui/ dosyaları gerçek Chromium'da, sahte
// Tauri IPC'si (mock-tauri.js) üzerinden çalışır. Kabuk ve sidecar gerekmez.
//
//   npm run test:ui
//
// Tarayıcı: `npx playwright install chromium`; başka bir Chromium için
// PW_CHROMIUM_PATH verilebilir.
import { test, before, after } from "node:test";
import assert from "node:assert/strict";
import http from "node:http";
import { readFile } from "node:fs/promises";
import path from "node:path";
import { fileURLToPath } from "node:url";
import { chromium } from "playwright";

const here = path.dirname(fileURLToPath(import.meta.url));
const uiDir = path.resolve(here, "../../ui");
const mockPath = path.join(here, "mock-tauri.js");
const TYPES = { ".html": "text/html", ".js": "text/javascript", ".css": "text/css", ".svg": "image/svg+xml", ".woff2": "font/woff2" };
const UUID = /[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}/i;

let server, base, browser;

before(async () => {
  server = http.createServer(async (req, res) => {
    const name = req.url === "/" ? "index.html" : decodeURIComponent(req.url.slice(1).split("?")[0]);
    const file = path.join(uiDir, name);
    if (!file.startsWith(uiDir)) return res.writeHead(403).end();
    try {
      const body = await readFile(file);
      res.writeHead(200, { "content-type": TYPES[path.extname(file)] || "application/octet-stream" }).end(body);
    } catch {
      res.writeHead(404).end();
    }
  });
  await new Promise((r) => server.listen(0, "127.0.0.1", r));
  base = `http://127.0.0.1:${server.address().port}/`;
  browser = await chromium.launch({ executablePath: process.env.PW_CHROMIUM_PATH || undefined });
});

after(async () => {
  await browser?.close();
  server?.close();
});

async function open() {
  const page = await browser.newPage();
  const errors = [];
  page.on("pageerror", (e) => errors.push(e.message));
  await page.addInitScript({ path: mockPath });
  await page.goto(base);
  return { page, errors };
}

/** Görünen metinde UUID ya da ham JSON olmamalı. */
async function assertClean(page, where) {
  const text = await page.evaluate(() => document.body.innerText);
  assert.doesNotMatch(text, UUID, `${where}: UUID görünüyor`);
  assert.doesNotMatch(text, /[{[]\s*"/, `${where}: ham JSON görünüyor`);
  assert.doesNotMatch(text, /\b[A-Z]{3,}_[A-Z_]{3,}\b/, `${where}: iç hata kodu görünüyor`);
}

async function login(page) {
  await page.getByRole("heading", { name: "HAKAN hesabınla gir" }).waitFor();
  await page.getByLabel("E-posta").fill("ayse@ornek.com");
  await page.locator("#password").fill("dogru");
  await page.getByRole("button", { name: "Devam" }).click();
  await page.getByLabel("Doğrulama uygulamasındaki 6 haneli kod").fill("123456");
  await page.getByRole("button", { name: "Devam" }).click();
  await page.getByRole("heading", { name: "Bu bilgisayarı tanıt" }).waitFor();
}

async function signIn(page) {
  await login(page);
  await page.getByRole("button", { name: "Kaydet" }).click();
  await page.getByRole("heading", { name: "İlk klasörü bağla" }).waitFor();
  await page.getByRole("button", { name: "Şimdilik atla" }).click();
  await page.getByRole("heading", { name: "İzlenen klasörler" }).waitFor();
}

const upload = (page, data) => page.evaluate((d) => window.__mock.emit("sidecar://event", { event: "upload", data: d }), data);
const ZEYTIN = "6f1d2e3c-4b5a-4c7d-8e9f-0a1b2c3d4e5f";

test("kurulum 1–2: Türkçe hata, iki adımlı doğrulama, ofis ve bilgisayar adı", async () => {
  const { page, errors } = await open();
  await page.getByRole("heading", { name: "HAKAN hesabınla gir" }).waitFor();
  assert.equal(await page.getByLabel("Ofisine verilen Hakan adresi").isVisible(), false, "sunucu adresi gizli 'gelişmiş'te");
  await page.getByLabel("E-posta").fill("ayse@ornek.com");
  await page.locator("#password").fill("yanlis");
  await page.getByRole("button", { name: "Devam" }).click();
  await page.getByText("E-posta ya da parola hatalı.").waitFor();
  await assertClean(page, "hatalı giriş");

  await page.locator("#password").fill("dogru");
  await page.getByRole("button", { name: "Devam" }).click();
  await page.getByText("Doğrulama kodunu girin").waitFor();
  await page.getByLabel("Doğrulama uygulamasındaki 6 haneli kod").fill("123456");
  await page.getByRole("button", { name: "Devam" }).click();

  await page.getByRole("heading", { name: "Bu bilgisayarı tanıt" }).waitFor();
  assert.equal(await page.getByRole("radio", { name: "Atlas YMM" }).isChecked(), true);
  assert.equal(await page.getByText("Kapanmış Ofis").count(), 0, "askıdaki ofis listelenmemeli");
  await assertClean(page, "ofis seçimi");
  await page.getByLabel("Bu bilgisayarın adı").fill("Muhasebe masası");
  await page.getByRole("button", { name: "Kaydet" }).click();
  await page.getByRole("heading", { name: "İlk klasörü bağla" }).waitFor();
  const use = await page.evaluate(() => window.__mock.calls.find((c) => c.args.method === "agent.use_organization"));
  assert.deepEqual(use.args.params, { slug: "atlas-ymm", agent_name: "Muhasebe masası" });
  assert.deepEqual(errors, []);
  await page.close();
});

test("kurulum 3: ilk klasör mükellef adıyla bağlanır, sonra ana pencere", async () => {
  const { page, errors } = await open();
  await login(page);
  await page.getByRole("button", { name: "Kaydet" }).click();
  await page.getByRole("heading", { name: "İlk klasörü bağla" }).waitFor();
  await page.getByRole("button", { name: "Bitir ve tepsiye geç" }).click();
  await page.getByText("Listeden bir mükellef seçin.").waitFor();
  await page.locator("#first-taxpayer").fill("Zeytin Gıda Ltd. Şti.");
  await page.getByRole("button", { name: "Bitir ve tepsiye geç" }).click();
  await page.getByText("İzlenecek klasörü seçin.").waitFor();
  await page.getByRole("button", { name: /Klasör seç ya da pencereye sürükle/ }).click();
  await page.getByText("/home/ofis/E-Faturalar/Zeytin").waitFor();
  await page.getByRole("button", { name: "Bitir ve tepsiye geç" }).click();
  await page.getByRole("heading", { name: "İzlenen klasörler" }).waitFor();
  const add = await page.evaluate(() => window.__mock.calls.find((c) => c.args.method === "watch.add"));
  assert.deepEqual(add.args.params, { path: "/home/ofis/E-Faturalar/Zeytin", engagement_uuid: ZEYTIN, label: "", taxpayer_title: "Zeytin Gıda Ltd. Şti." });
  assert.equal(await page.locator("#office-name").textContent(), "Atlas YMM");
  await page.locator("#agent-card").getByText("Muhasebe masası").waitFor();
  assert.deepEqual(errors, []);
  await page.close();
});

test("klasörler: yol yazmadan, mükellef adıyla; durum, menü ve kaldırma", async () => {
  const { page, errors } = await open();
  await signIn(page);
  await page.getByRole("heading", { name: "Henüz izlenen klasör yok" }).waitFor();
  assert.equal(await page.locator("input#watch-path").count(), 0, "elle yol alanı olmamalı");

  await page.getByRole("button", { name: "+ Klasör ekle" }).first().click();
  const options = await page.locator("#engagement option").allTextContents();
  assert.deepEqual(options, ["Ahmet Yılmaz", "Zeytin Gıda Ltd. Şti."], "mükellefler adlarıyla, alfabetik");
  await page.getByPlaceholder("Mükellef adıyla arayın").fill("zeytin");
  assert.deepEqual(await page.locator("#engagement option").allTextContents(), ["Zeytin Gıda Ltd. Şti."]);

  await page.getByRole("button", { name: "İzlemeye al" }).click();
  await page.getByText("İzlenecek klasörü seçin.").waitFor();
  await page.getByRole("button", { name: /Klasör seç…/ }).click();
  await page.getByText("/home/ofis/E-Faturalar/Zeytin").waitFor();
  await page.getByLabel("Kısa ad (isteğe bağlı)").fill("Gelen e-faturalar");
  await page.getByRole("button", { name: "İzlemeye al" }).click();

  const row = page.locator("#watches .fl").first();
  await row.getByText("Zeytin Gıda Ltd. Şti. · Gelen e-faturalar").waitFor();
  await row.getByText("1 dosyada sorun").waitFor();
  await row.getByText("5 dk önce · 2").waitFor();
  assert.equal(await page.locator("#cnt-folders").textContent(), "1");
  assert.equal(await page.locator("#cnt-problems").textContent(), "1");
  const add = await page.evaluate(() => window.__mock.calls.find((c) => c.args.method === "watch.add"));
  assert.deepEqual(add.args.params, { path: "/home/ofis/E-Faturalar/Zeytin", engagement_uuid: ZEYTIN, label: "Gelen e-faturalar", taxpayer_title: "Zeytin Gıda Ltd. Şti." });
  await assertClean(page, "klasörler");

  await row.getByRole("button", { name: "Zeytin Gıda Ltd. Şti.: seçenekler" }).click();
  await row.getByRole("button", { name: "İzlemeden çıkar" }).click();
  await page.getByRole("dialog").getByRole("button", { name: "İzlemeden çıkar" }).click();
  await page.getByRole("heading", { name: "Henüz izlenen klasör yok" }).waitFor();
  assert.deepEqual(errors, []);
  await page.close();
});

test("geçmiş ve sorunlar: geçici / kalıcı ayrımı, ham hata yok", async () => {
  const { page, errors } = await open();
  await signIn(page);
  await page.getByRole("button", { name: "+ Klasör ekle" }).first().click();
  await page.getByPlaceholder("Mükellef adıyla arayın").fill("zeytin");
  await page.getByRole("button", { name: /Klasör seç…/ }).click();
  await page.getByRole("button", { name: "İzlemeye al" }).click();
  await page.locator("#watches .fl").first().waitFor();

  await upload(page, { path: "/x/a.xml", name: "fatura-003.xml", engagement_uuid: ZEYTIN, outcome: "uploaded", job_uuid: "d3adb33f-0000-4000-8000-000000000000" });
  await upload(page, { path: "/x/b.xml", name: "fatura-004.xml", engagement_uuid: ZEYTIN, outcome: "failed", error: 'Post "https://x": dial tcp: connection refused', reason: "Sunucuya gönderilemedi; bir sonraki taramada yeniden denenecek." });
  await upload(page, { path: "/x/c.xml", name: "fatura-001.xml", engagement_uuid: ZEYTIN, outcome: "duplicate" });

  await page.getByRole("tab", { name: "Geçmiş" }).click();
  const table = page.locator("#history-table");
  await table.getByText("fatura-003.xml").waitFor();
  await table.getByText("Sunucuya gönderilemedi; bir sonraki taramada yeniden denenecek.").waitFor();
  await table.getByText("Zaten var").waitFor();
  assert.equal(await table.getByText("Zeytin Gıda Ltd. Şti.").count(), 3);
  assert.equal(await table.getByText("connection refused").count(), 0, "ham hata gösterilmemeli");
  await page.getByText("Bugün 1 dosya · son 7 gün 1").waitFor();
  await page.getByLabel("Dosya ya da mükellef ara").fill("004");
  assert.equal(await table.locator("tbody tr").count(), 1);
  await assertClean(page, "geçmiş");

  // Sorunlar: mock'taki hata kalıcı (retry: false); bir geçici hata ekle.
  await page.evaluate(() => {
    window.__mock.watches[0].activity.errors.unshift({ name: "fatura-004.xml", message: "Sunucuya gönderilemedi; bir sonraki taramada yeniden denenecek.", retry: true, at: new Date().toISOString() });
  });
  await page.getByRole("tab", { name: "Sorunlar" }).click();
  await page.evaluate(() => window.__mock.emit("sidecar://event", { event: "agent.heartbeat", data: {} }));
  const box = page.locator("#problems");
  await box.getByText("Geçici · kendiliğinden yeniden denenir").waitFor();
  await box.getByText(/Sonraki taramada otomatik yeniden denenecek/).waitFor();
  await box.getByRole("progressbar", { name: "Sonraki deneme" }).waitFor();
  await box.getByText("Kalıcı · senin kararını bekler").waitFor();
  await box.getByText("bozuk.xml · Zeytin Gıda Ltd. Şti.").waitFor();
  await box.getByText("1 dosya · zaten var").waitFor();
  assert.equal(await page.locator("#cnt-problems").textContent(), "2");
  await assertClean(page, "sorunlar");

  await box.getByRole("button", { name: "Yok say" }).click();
  await box.getByText("bozuk.xml · Zeytin Gıda Ltd. Şti.").waitFor({ state: "detached" });
  assert.equal(await page.locator("#cnt-problems").textContent(), "1");
  assert.deepEqual(errors, []);
  await page.close();
});

test("tepsi menüsü sekme ister (#325): kurulumda yok sayılır, sonra sekme açılır", async () => {
  const { page, errors } = await open();
  const nav = (tab) => page.evaluate((t) => window.__mock.emit("shell://navigate", t), tab);
  await login(page);
  await nav("problems");
  assert.equal(await page.locator("#view-problems").isHidden(), true, "kurulumda sekme açılmamalı");
  await page.getByRole("button", { name: "Kaydet" }).click();
  await page.getByRole("heading", { name: "İlk klasörü bağla" }).waitFor();
  await page.getByRole("button", { name: "Şimdilik atla" }).click();
  await page.getByRole("heading", { name: "İzlenen klasörler" }).waitFor();
  await nav("problems");
  await page.locator("#view-problems").waitFor({ state: "visible" });
  assert.equal(await page.getByRole("tab", { name: "Sorunlar" }).getAttribute("aria-selected"), "true");
  await nav("history");
  await page.locator("#view-history").waitFor({ state: "visible" });
  await nav("settings");
  await page.locator("#view-settings").waitFor({ state: "visible" });
  await nav("bilinmeyen");
  await page.locator("#view-settings").waitFor({ state: "visible" });
  assert.deepEqual(errors, []);
  await page.close();
});

test("muhasebe programı, ayarlar ve gelişmiş: okunur sonuçlar", async () => {
  const { page, errors } = await open();
  await signIn(page);

  await page.getByRole("tab", { name: "Muhasebe programı" }).click();
  await page.getByLabel("Sunucu", { exact: true }).fill("MUHASEBE-PC");
  await page.getByLabel("Veritabanı").fill("LOGODB");
  await page.getByLabel("Kullanıcı").fill("okur");
  await page.locator("#src-pass").fill("x");
  await page.getByRole("button", { name: "Bağlantıyı dene" }).click();
  await page.getByText("Beklenen 3 tablonun 2 tanesi bulundu.").waitFor();
  await page.getByText("LG_001_CLCARD").waitFor();
  await page.getByRole("button", { name: "Geçen ayı önizle" }).click();
  const table = page.locator("table.grid");
  await table.getByText("Deniz Market").waitFor();
  await table.getByText("1.180,50").waitFor();
  assert.equal(await table.getByText("Satış").count(), 1);
  const resultText = await page.locator("#src-result").innerText();
  assert.doesNotMatch(resultText, UUID);
  assert.doesNotMatch(resultText, /LOGICALREF|[{}]/);
  await page.getByRole("radio", { name: "Mikro" }).check();
  assert.equal(await page.getByLabel("Firma no").isVisible(), false, "Mikro'da firma no yok");

  await page.getByRole("tab", { name: "Ayarlar" }).click();
  await page.getByText("Bu derlemede otomatik güncelleme kapalı: imza anahtarı tanımlanmamış.").waitFor();
  assert.equal(await page.getByLabel("Kanal").isDisabled(), true);
  await page.getByRole("switch", { name: "Klasör izlemeyi duraklat" }).check();
  await page.getByText("Klasör izleme duraklatıldı.").waitFor();
  await page.locator("#agent-card").getByText("Duraklatıldı").waitFor();
  await page.getByRole("button", { name: "İzlemeyi sürdür" }).click();
  await page.getByText("Klasör izleme duraklatıldı.").waitFor({ state: "hidden" });

  // Güncelleme hazır: kenar menüde nokta, ayarlarda kart, üstte şerit.
  await page.evaluate(() =>
    window.__mock.emit("updater://status", { enabled: true, current: "0.2.0", channel: "stable", state: "available", available: "0.3.0" }),
  );
  await page.getByText("Hakan 0.3.0 sürümü hazır.").waitFor();
  await page.locator("#up-badge").getByText("0.3.0 hazır").waitFor();
  assert.equal(await page.locator("#cnt-settings").textContent(), "•");
  await page.evaluate(() => window.__mock.emit("updater://status", { enabled: true, current: "0.2.0", channel: "stable", state: "downloading", available: "0.3.0" }));
  await page.evaluate(() => window.__mock.emit("updater://progress", { downloaded: 524288, total: 1048576 }));
  await page.getByText("512 KB / 1.0 MB (%50)").waitFor();

  // Görünüm: palet ve kip seçimi tema'yı değiştirir.
  await page.getByRole("radio", { name: "Yeşil" }).check();
  await page.getByRole("radio", { name: "Koyu" }).check();
  assert.equal(await page.evaluate(() => document.documentElement.dataset.theme), "n3-dark");

  await page.getByText("Gelişmiş", { exact: true }).click();
  await page.getByText("Çalışıyor (sürüm 0.2.0)").waitFor();
  await page.getByRole("button", { name: "Yerel ajanı dene" }).click();
  await page.getByText("Yerel ajan yanıt verdi").waitFor();
  await assertClean(page, "ayarlar");
  assert.deepEqual(errors, []);
  await page.close();
});

test("sekmeler klavyeyle gezilir; dar pencerede ikon rayı", async () => {
  const { page, errors } = await open();
  await signIn(page);
  await page.getByRole("tab", { name: "Klasörler" }).focus();
  await page.keyboard.press("ArrowDown");
  assert.equal(await page.getByRole("tab", { name: "Geçmiş" }).getAttribute("aria-selected"), "true");
  await page.keyboard.press("End");
  assert.equal(await page.getByRole("tab", { name: "Ayarlar" }).getAttribute("aria-selected"), "true");
  await page.setViewportSize({ width: 720, height: 600 });
  const w = await page.locator("aside.side").evaluate((n) => n.getBoundingClientRect().width);
  assert.equal(w, 64, "768'in altında ikon rayı");
  await page.setViewportSize({ width: 480, height: 700 });
  const h = await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth);
  assert.ok(h, "dar pencerede yatay kaydırma olmamalı");
  assert.deepEqual(errors, []);
  await page.close();
});

test("arayüz v1 temeli (#324): tema, yazı tipi ve ortak bileşenler", async () => {
  const { page, errors } = await open();
  // Varsayılan M3 + sistemi izle.
  assert.equal(await page.evaluate(() => document.documentElement.dataset.theme), "m3-light");
  await page.emulateMedia({ colorScheme: "dark" });
  await page.waitForFunction(() => document.documentElement.dataset.theme === "m3-dark");
  const bg = await page.evaluate(() => getComputedStyle(document.body).backgroundColor);
  assert.equal(bg, "rgb(10, 10, 10)");
  await page.evaluate(async () => (await import("./tema.js")).temaSec({ palet: "n3", kip: "light" }));
  assert.equal(await page.evaluate(() => document.documentElement.dataset.theme), "n3-light");

  await page.evaluate(() => document.fonts.ready);
  assert.ok(await page.evaluate(() => document.fonts.check("13px Inter")), "Inter yüklenmeli");

  const out = await page.evaluate(async () => {
    const ui = await import("./hakan-ui.js");
    const tac = ui.TacRozeti({ durum: "onaylandi" });
    const etiketli = ui.TacRozeti({ durum: "sorun", boyut: 30, etiket: true, metin: "Okunamadı" });
    const guven = ui.GuvenOlcegi({ deger: 0.42, kaynak: "kural" });
    const yukleyici = ui.KristalYukleyici({ gecikme: 0 });
    const bos = ui.BosDurum({ baslik: "Kuyruk boş, taç tamam" });
    document.body.append(tac, etiketli, guven, yukleyici, bos);
    return {
      tac: tac.getAttribute("aria-label"),
      sagDolu: getComputedStyle(tac.children[2]).backgroundColor,
      etiket: etiketli.textContent,
      kademe: guven.dataset.kademe,
      guvenAd: guven.getAttribute("aria-label"),
      yukleyici: yukleyici.getAttribute("role"),
      kademeler: [0.59, 0.6, 0.9].map((d) => ui.guvenKademesi(d)),
    };
  });
  assert.equal(out.tac, "Müşavir onayladı");
  assert.equal(out.sagDolu, "rgb(10, 10, 10)"); // n3-light --c-dark
  assert.equal(out.etiket, "Okunamadı");
  assert.equal(out.kademe, "yuksek"); // kural eşleşmesi her zaman yüksek
  assert.match(out.guvenAd, /^Yüksek güven · %42 · kural eşleşmesi$/);
  assert.equal(out.yukleyici, "status");
  assert.deepEqual(out.kademeler, ["dusuk", "orta", "yuksek"]);
  await page.getByRole("heading", { name: "Kuyruk boş, taç tamam" }).waitFor();
  assert.deepEqual(errors, []);
  await page.close();
});

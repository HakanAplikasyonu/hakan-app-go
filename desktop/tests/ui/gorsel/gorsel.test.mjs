// Masaüstü ajanı görsel regresyon testi (#332): ana ekranlar dört temada
// (m3-light, m3-dark, n3-light, n3-dark) ve üç pencere genişliğinde temel
// görüntüyle karşılaştırılır. ui/ dosyaları gerçek Chromium'da, sahte Tauri
// IPC'si (../mock-tauri.js) ile çalışır; kabuk ve sidecar gerekmez.
//
//   npm run test:visual                       karşılaştır
//   GORSEL_GUNCELLE=1 npm run test:visual     temel görüntüleri yenile
//
// Temel görüntüler: tests/ui/gorsel/temel/<ekran>-<tema>-<genişlik>.png.
// Fark çıkarsa gerçek görüntü ve fark haritası tests/ui/gorsel/sonuc/
// altına yazılır (CI bunu artefakt olarak yükler).
//
// Karşılaştırma ek bağımlılık olmadan tarayıcıda yapılır: iki PNG canvas'a
// çizilir, kanal farkı TOLERANS'tan büyük piksellerin oranı ORAN'ı aşarsa
// test düşer. Saat sabit (page.clock), hareket azaltılmış, imleç ve geçişler
// kapalı, yazı tipleri yüklendikten sonra çekilir.
import { test, before, after } from "node:test";
import assert from "node:assert/strict";
import http from "node:http";
import { readFile, writeFile, mkdir, rm } from "node:fs/promises";
import { existsSync } from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";
import { chromium } from "playwright";

const here = path.dirname(fileURLToPath(import.meta.url));
const uiDir = path.resolve(here, "../../../ui");
const mockPath = path.resolve(here, "../mock-tauri.js");
const baseDir = path.join(here, "temel");
const outDir = path.join(here, "sonuc");
const UPDATE = process.env.GORSEL_GUNCELLE === "1";
const TYPES = { ".html": "text/html", ".js": "text/javascript", ".css": "text/css", ".svg": "image/svg+xml", ".woff2": "font/woff2" };

/** Dört tema. */
const THEMES = [
  { name: "m3-light", palet: "m3", kip: "light" },
  { name: "m3-dark", palet: "m3", kip: "dark" },
  { name: "n3-light", palet: "n3", kip: "light" },
  { name: "n3-dark", palet: "n3", kip: "dark" },
];

/**
 * Pencere genişlikleri: ajan penceresinin kırılımları (#342): ≥ 900 tam
 * menü, 600–899 ikon rayı, < 600 üst sekme şeridi. Pencerenin en küçük
 * genişliği 720, varsayılanı 960.
 */
const SIZES = [
  { name: "genis", width: 1140, height: 760 },
  { name: "orta", width: 720, height: 640 },
  { name: "dar", width: 480, height: 800 },
];

/** Sabit saat: göreli zamanlar ("2 dk önce") her koşuda aynı olsun. */
const NOW = new Date("2026-11-16T10:42:00+03:00");

/** Bir kanaldaki bu kadarlık fark yok sayılır (kenar yumuşatma). */
const TOLERANS = 24;
/** Farklı piksel oranı bunu aşarsa test düşer (%0,02). Palet farkı (m3 ↔ n3)
 * bazı ekranlarda yalnızca %0,04 tutar; sınır bunun altında kalmalı. */
const ORAN = 0.0002;

/** Kurulmuş ajan: oturum açık, ofis seçili, iki klasör (biri sorunlu). */
function seedInstalled() {
  const m = window.__mock;
  const at = (min) => new Date(Date.now() - min * 60000).toISOString();
  m.signedIn = true;
  m.running = true;
  m.watches = [
    {
      path: "C:\\E-Faturalar\\Zeytin",
      engagement_uuid: "6f1d2e3c-4b5a-4c7d-8e9f-0a1b2c3d4e5f",
      label: "Gelen e-faturalar",
      taxpayer_title: "Zeytin Gıda Ltd. Şti.",
      activity: { uploaded: 12, last_file: "KLM2026000184213.xml", last_outcome: "uploaded", last_upload_at: at(2), last_at: at(2), errors: [] },
    },
    {
      path: "C:\\E-Faturalar\\Yilmaz",
      engagement_uuid: "7a2b3c4d-5e6f-4a7b-8c9d-0e1f2a3b4c5d",
      label: "",
      taxpayer_title: "Ahmet Yılmaz",
      activity: {
        uploaded: 3,
        last_file: "tarama_0091.pdf",
        last_outcome: "rejected",
        last_upload_at: at(40),
        last_at: at(4),
        errors: [
          { name: "TKN2026000391827.xml", message: "Sunucuya gönderilemedi; bir sonraki taramada yeniden denenecek.", retry: true, at: at(1) },
          { name: "tarama_0091.pdf", message: "Dosya e-fatura (UBL-TR) değil; yüklenmedi.", retry: false, at: at(4) },
        ],
      },
    },
  ];
}

/** Ekranlar: ad, hazırlık (init script) ve ekrana gitme. */
const SCREENS = [
  {
    name: "kurulum-giris",
    seed: null,
    go: (page) => page.getByRole("heading", { name: "HAKAN hesabınla gir" }).waitFor(),
  },
  {
    name: "klasorler",
    seed: seedInstalled,
    go: (page) => page.getByRole("heading", { name: "İzlenen klasörler" }).waitFor(),
  },
  {
    name: "sorunlar",
    seed: seedInstalled,
    go: async (page) => {
      await page.getByRole("heading", { name: "İzlenen klasörler" }).waitFor();
      await page.locator(`#tabs [data-view="problems"]`).click();
      await page.locator("#view-problems").waitFor({ state: "visible" });
      await page.locator("#problems").getByText("tarama_0091.pdf", { exact: false }).first().waitFor();
    },
  },
  {
    name: "ayarlar",
    seed: seedInstalled,
    go: async (page) => {
      await page.getByRole("heading", { name: "İzlenen klasörler" }).waitFor();
      await page.locator(`#tabs [data-view="settings"]`).click();
      await page.locator("#view-settings").waitFor({ state: "visible" });
    },
  },
];

/** Geçişleri, imleci ve kaydırma çubuklarını kapatır. */
const FREEZE_CSS = `*,*::before,*::after{animation:none!important;transition:none!important;caret-color:transparent!important}
::-webkit-scrollbar{display:none}`;

let server, base, browser, cmpPage;

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
  cmpPage = await browser.newPage();
  await rm(outDir, { recursive: true, force: true });
});

after(async () => {
  await browser?.close();
  server?.close();
});

/** İki PNG'yi tarayıcıda karşılaştırır; fark oranı ve fark haritası döner. */
async function compare(actual, expected) {
  return cmpPage.evaluate(
    async ({ a, e, tol }) => {
      const load = async (b64) => createImageBitmap(await (await fetch(`data:image/png;base64,${b64}`)).blob());
      const [ia, ie] = await Promise.all([load(a), load(e)]);
      if (ia.width !== ie.width || ia.height !== ie.height) {
        return { ratio: 1, reason: `boyut farklı: ${ia.width}×${ia.height} ≠ ${ie.width}×${ie.height}` };
      }
      const { width: w, height: h } = ia;
      const data = (img) => {
        const c = new OffscreenCanvas(w, h);
        const ctx = c.getContext("2d");
        ctx.drawImage(img, 0, 0);
        return ctx.getImageData(0, 0, w, h).data;
      };
      const da = data(ia);
      const de = data(ie);
      const diff = new OffscreenCanvas(w, h);
      const dctx = diff.getContext("2d");
      const out = dctx.createImageData(w, h);
      let bad = 0;
      for (let i = 0; i < da.length; i += 4) {
        const off =
          Math.abs(da[i] - de[i]) > tol ||
          Math.abs(da[i + 1] - de[i + 1]) > tol ||
          Math.abs(da[i + 2] - de[i + 2]) > tol ||
          Math.abs(da[i + 3] - de[i + 3]) > tol;
        if (off) bad++;
        out.data[i] = off ? 255 : de[i] * 0.2 + 200;
        out.data[i + 1] = off ? 0 : de[i + 1] * 0.2 + 200;
        out.data[i + 2] = off ? 64 : de[i + 2] * 0.2 + 200;
        out.data[i + 3] = 255;
      }
      if (!bad) return { ratio: 0 };
      dctx.putImageData(out, 0, 0);
      const blob = await diff.convertToBlob({ type: "image/png" });
      const buf = new Uint8Array(await blob.arrayBuffer());
      let bin = "";
      for (let i = 0; i < buf.length; i += 0x8000) bin += String.fromCharCode(...buf.subarray(i, i + 0x8000));
      return { ratio: bad / (w * h), diff: btoa(bin) };
    },
    { a: actual.toString("base64"), e: expected.toString("base64"), tol: TOLERANS },
  );
}

async function shoot(screen, theme, size) {
  const ctx = await browser.newContext({
    viewport: { width: size.width, height: size.height },
    deviceScaleFactor: 1,
    reducedMotion: "reduce",
    colorScheme: theme.kip,
    locale: "tr-TR",
    timezoneId: "Europe/Istanbul",
  });
  const page = await ctx.newPage();
  const errors = [];
  page.on("pageerror", (e) => errors.push(e.message));
  await page.clock.setFixedTime(NOW);
  await page.addInitScript(({ palet, kip }) => {
    localStorage.setItem("hakan.palet", palet);
    localStorage.setItem("hakan.kip", kip);
  }, theme);
  await page.addInitScript({ path: mockPath });
  if (screen.seed) await page.addInitScript(screen.seed);
  await page.goto(base);
  await page.addStyleTag({ content: FREEZE_CSS });
  await screen.go(page);
  await page.evaluate(() => document.fonts.ready);
  assert.equal(await page.evaluate(() => document.documentElement.dataset.theme), theme.name, "tema uygulanmadı");
  // Odak halkası ve fare üstü durumu çekime girmesin.
  await page.mouse.move(0, 0);
  await page.evaluate(() => document.activeElement?.blur());
  const png = await page.screenshot({ animations: "disabled", caret: "hide" });
  await ctx.close();
  assert.deepEqual(errors, [], "sayfa hatası");
  return png;
}

for (const screen of SCREENS) {
  for (const theme of THEMES) {
    for (const size of SIZES) {
      const name = `${screen.name}-${theme.name}-${size.name}`;
      test(`görsel: ${name}`, async (t) => {
        const actual = await shoot(screen, theme, size);
        const file = path.join(baseDir, `${name}.png`);
        if (UPDATE || !existsSync(file)) {
          await mkdir(baseDir, { recursive: true });
          await writeFile(file, actual);
          t.diagnostic(UPDATE ? "temel görüntü yenilendi" : "temel görüntü yoktu, yazıldı");
          return;
        }
        const res = await compare(actual, await readFile(file));
        if (res.ratio > ORAN) {
          await mkdir(outDir, { recursive: true });
          await writeFile(path.join(outDir, `${name}-gercek.png`), actual);
          if (res.diff) await writeFile(path.join(outDir, `${name}-fark.png`), Buffer.from(res.diff, "base64"));
        }
        assert.ok(
          res.ratio <= ORAN,
          `${name}: piksellerin %${(res.ratio * 100).toFixed(2)}'i farklı (sınır %${(ORAN * 100).toFixed(2)})${res.reason ? `; ${res.reason}` : ""}. ` +
            `Bilerek değiştiyse: GORSEL_GUNCELLE=1 npm run test:visual`,
        );
      });
    }
  }
}

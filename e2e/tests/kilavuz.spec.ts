import { mkdirSync, readdirSync, readFileSync } from "node:fs";
import path from "node:path";

import { expect, test, type Page } from "@playwright/test";

import { Api, DEMO, engagementOf, officePath, uiLogin } from "./lib";

/**
 * Kullanım kılavuzları (#274, docs/kullanim/).
 *
 * "@docs" testleri kılavuzlardaki ekran görüntülerini demo verisinden
 * yeniden üretir: frontend/public/yardim/img/*.jpg. Akış testleriyle birlikte
 * koşmaz; elle ya da CI'daki "Kılavuz ekran görüntüleri" adımıyla:
 *
 *   npx playwright test --grep @docs
 *
 * Ofis görüntülerinden önce demo UBL seti (backend/internal/demo/ubl) Atlas
 * Gıda'ya API ile yüklenir; aynı ETTN ikinci kez eklenmediği için akış
 * testlerinden sonra koşması sorun değildir. Ortamı sıfırdan kurmadan önce
 * koşarsa smoke testinin sayımları tutmaz: önce akışlar, sonra @docs.
 *
 * Yardım sayfası testi (etiketsiz) her koşuda çalışır: bağlantılar ve sayfalar.
 */

const IMG_DIR = path.resolve(__dirname, "../../frontend/public/yardim/img");
const VIEWPORT = { width: 1280, height: 800 };

async function shot(page: Page, name: string) {
  mkdirSync(IMG_DIR, { recursive: true });
  // Sorgular ve animasyonlar dinsin; dev hata ayıklama paneli üretimde yok.
  await page.waitForLoadState("networkidle");
  await page.evaluate(() => document.fonts.ready);
  await page.waitForTimeout(400);
  await page.screenshot({
    path: path.join(IMG_DIR, `${name}.jpg`),
    type: "jpeg",
    quality: 80,
    animations: "disabled",
    caret: "hide",
  });
}

test.describe("@docs ofis paneli görüntüleri", () => {
  test.use({ viewport: VIEWPORT, colorScheme: "light" });
  let atlas = "";
  test.beforeAll(async () => {
    const api = await Api.login(DEMO.owner);
    atlas = await engagementOf(api, DEMO.atlas.title);
    const files = readdirSync(DEMO.ublDir).filter((f) => f.startsWith("atlas-") && f.endsWith(".xml"));
    for (const f of files) {
      const res = await api.upload(`/v1/tenant/engagements/${atlas}/ubl-uploads`, {
        name: f,
        mimeType: "application/xml",
        buffer: readFileSync(path.join(DEMO.ublDir, f)),
      });
      expect(res.status(), `${f}: ${await res.text()}`).toBeLessThan(300);
    }
    await expect
      .poll(
        async () => {
          const jobs = await api.json<{ items: { status: string }[] }>(
            await api.get(`/v1/tenant/engagements/${atlas}/ubl-uploads?limit=50`),
          );
          return jobs.items.length > 0 && jobs.items.every((j) => j.status === "done");
        },
        { timeout: 90_000 },
      )
      .toBe(true);
    // Masaüstü ajanları sayfası (#285) için iki örnek ajan (aynı makine
    // kimliği yeniden kaydı günceller, koşular çoğalmaz).
    for (const [machine_id, name, platform] of [
      ["kilavuz-pc-0001", "Muhasebe-PC-01", "windows"],
      ["kilavuz-mac-0002", "Ortak-Mac", "macos"],
    ]) {
      const res = await api.post("/v1/tenant/agents", { machine_id, name, platform, version: "0.1.0" });
      expect(res.status(), await res.text()).toBe(200);
    }
    await api.dispose();
  });

  test("ofis ekranları", async ({ page }) => {
    await uiLogin(page, DEMO.owner);
    const screens: [string, string, () => Promise<void>][] = [
      ["ofis-genel-bakis", "", () => expect(page.getByText(DEMO.atlas.title).first()).toBeVisible()],
      ["ofis-komuta-merkezi", "/command", () => expect(page.getByText(DEMO.atlas.title).first()).toBeVisible()],
      ["ofis-mukellefler", "/taxpayers", () => expect(page.getByText(DEMO.atlas.title).first()).toBeVisible()],
      ["ofis-mukellef-ekle", "/taxpayer-requests", () => expect(page.getByRole("heading").first()).toBeVisible()],
      ["ofis-uyeler", "/members", () => expect(page.getByText(DEMO.owner).first()).toBeVisible()],
      ["ofis-ubl-yukle", `/upload?taxpayer=${atlas}`, () => expect(page.getByRole("heading").first()).toBeVisible()],
      ["ofis-faturalar", `/invoices?taxpayer=${atlas}`, () => expect(page.getByRole("table").first()).toBeVisible()],
      ["ofis-inceleme-kuyrugu", "/review", () => expect(page.getByRole("heading").first()).toBeVisible()],
      ["ofis-uyarilar", "/warnings", () => expect(page.getByRole("heading").first()).toBeVisible()],
      ["ofis-yevmiye", `/journals?taxpayer=${atlas}`, () => expect(page.getByRole("heading").first()).toBeVisible()],
      ["ofis-masaustu-ajanlari", "/agents", () => expect(page.getByText("Muhasebe-PC-01").first()).toBeVisible()],
    ];
    for (const [name, p, ready] of screens) {
      await test.step(name, async () => {
        await page.goto(officePath(p));
        await ready();
        await shot(page, name);
      });
    }
    await test.step("ofis-komut-paleti", async () => {
      await page.goto(officePath());
      await page.waitForLoadState("networkidle");
      await page.keyboard.press("Control+k");
      await expect(page.getByRole("dialog")).toBeVisible();
      await page.keyboard.type("Atlas");
      await shot(page, "ofis-komut-paleti");
    });
  });
});

test.describe("@docs mükellef paneli görüntüleri", () => {
  test.use({ viewport: VIEWPORT, colorScheme: "light" });
  test("mükellef ekranları", async ({ page }) => {
    await uiLogin(page, DEMO.taxpayerUser);
    await page.waitForURL(/\/m\/[0-9a-f-]{36}/);
    const base = new URL(page.url()).pathname;
    for (const [name, p] of [
      ["mukellef-ozet", ""],
      ["mukellef-faturalar", "/invoices"],
      ["mukellef-fisler", "/receipts"],
      ["mukellef-belge-yukle", "/uploads"],
    ] as const) {
      await test.step(name, async () => {
        await page.goto(base + p);
        await expect(page.getByRole("navigation", { name: "Mükellef paneli bölümleri" })).toBeVisible();
        await shot(page, name);
      });
    }
  });
});

/**
 * Masaüstü penceresi: desktop/ui statik arayüzü, Tauri yerine sahte bir
 * köprüyle açılır. Değerler örnektir (gerçek ajan, ofis ya da makine yok).
 */
const tauriMock = (signedIn: boolean) => `
  (() => {
    const status = ${JSON.stringify({
      signed_in: true,
      running: true,
      base_url: "https://api.ornek-ofis.com",
      organization: { name: "Demo SMMM Ofisi" },
      agent_name: "Muhasebe-PC-01",
      last_heartbeat: "2026-11-16T09:30:00+03:00",
      uploaded_total: 128,
      watches: [
        {
          path: "C:\\Faturalar\\Atlas",
          label: "Atlas Gıda indirilenler",
          engagement_uuid: "7c2f9a1e-0000-4000-8000-000000000001",
          taxpayer_title: "Atlas Gıda Ltd. Şti.",
          activity: { uploaded: 12, last_file: "ATL2026000000413.xml", last_outcome: "rejected", last_upload_at: "2026-11-16T09:28:00+03:00", errors: [
            { name: "notlar.xml", message: "Dosya e-fatura (UBL-TR) değil; yüklenmedi.", retry: false, at: "2026-11-16T09:29:00+03:00" },
          ] },
        },
        {
          path: "C:\\Faturalar\\Selin",
          label: "Selin Yılmaz e-arşiv",
          engagement_uuid: "3b8d4c2a-0000-4000-8000-000000000002",
          taxpayer_title: "Selin Yılmaz",
          activity: { uploaded: 4, last_file: "SLN2026000000021.xml", last_outcome: "uploaded", last_upload_at: "2026-11-16T09:10:00+03:00", errors: [] },
        },
      ],
    })};
    const signedIn = ${signedIn};
    const handlers = {};
    const answer = (method) => {
      switch (method) {
        case "agent.status":
          return signedIn ? status : { signed_in: false, running: false, base_url: "", watches: [] };
        case "agent.engagements":
          return { items: [
            { uuid: "7c2f9a1e-0000-4000-8000-000000000001", taxpayer_uuid: "a1b2c3d4-0000-4000-8000-00000000000a", taxpayer_title: "Atlas Gıda Ltd. Şti." },
            { uuid: "3b8d4c2a-0000-4000-8000-000000000002", taxpayer_uuid: "e5f6a7b8-0000-4000-8000-00000000000b", taxpayer_title: "Selin Yılmaz" },
          ] };
        default:
          return {};
      }
    };
    window.__TAURI__ = {
      core: {
        invoke: async (cmd, args) => {
          if (cmd === "sidecar_call") return answer(args.method);
          if (cmd === "sidecar_status") return { ready: { version: "0.1.0", commit: "demo", os: "windows", arch: "amd64", pid: 4242 } };
          if (cmd === "shell_settings") return { autostart: true, paused: false };
          if (cmd === "update_status") return { enabled: true, current: "0.1.0", channel: "stable", state: "up_to_date" };
          return {};
        },
      },
      event: {
        listen: async (name, fn) => { (handlers[name] ||= []).push(fn); return () => {}; },
      },
      app: { getVersion: async () => "0.1.0" },
    };
    window.__emit = (name, payload) => (handlers[name] || []).forEach((fn) => fn({ payload }));
  })();
`;

test.describe("@docs masaüstü görüntüleri", () => {
  test.use({ viewport: { width: 1100, height: 900 }, colorScheme: "light" });
  // type="module" betikler file:// altında yüklenmez: arayüzü sahte bir
  // http kökünden diskteki dosyalarla sun.
  const UI_DIR = path.resolve(__dirname, "../../desktop/ui");
  const ui = "http://desktop.hakan.test/index.html";
  const types: Record<string, string> = {
    ".html": "text/html",
    ".js": "text/javascript",
    ".css": "text/css",
    ".svg": "image/svg+xml",
    ".woff2": "font/woff2",
  };
  test.beforeEach(async ({ page }) => {
    await page.route("http://desktop.hakan.test/**", (route) => {
      // Alt klasörler (fonts/) korunur; kök dışına çıkan yol reddedilir.
      const rel = decodeURIComponent(new URL(route.request().url()).pathname).replace(/^\/+/, "") || "index.html";
      const file = path.resolve(UI_DIR, rel);
      if (!file.startsWith(UI_DIR + path.sep)) return route.abort();
      return route.fulfill({ body: readFileSync(file), contentType: types[path.extname(file)] ?? "application/octet-stream" });
    });
  });

  test("oturum açılmamış", async ({ page }) => {
    await page.addInitScript(tauriMock(false));
    await page.goto(ui);
    await expect(page.locator("#agent-badge")).toHaveText("Oturum açılmadı");
    await shot(page, "masaustu-oturum");
  });

  test("klasör izleme çalışıyor", async ({ page }) => {
    await page.addInitScript(tauriMock(true));
    await page.goto(ui);
    await expect(page.locator("#agent-badge")).toHaveText("Çevrimiçi");
    await page.evaluate(() => {
      const emit = (window as unknown as { __emit: (n: string, p: unknown) => void }).__emit;
      const eng = "7c2f9a1e-0000-4000-8000-000000000001";
      emit("sidecar://event", { event: "upload", data: { name: "ATL2026000000412.xml", engagement_uuid: eng, outcome: "uploaded" } });
      emit("sidecar://event", { event: "upload", data: { name: "ATL2026000000413.xml", engagement_uuid: eng, outcome: "uploaded" } });
      emit("sidecar://event", { event: "upload", data: { name: "notlar.xml", engagement_uuid: eng, outcome: "rejected", reason: "Dosya e-fatura (UBL-TR) değil; yüklenmedi." } });
    });
    await shot(page, "masaustu-izleme");
  });
});

test.describe("yardım sayfaları (#274)", () => {
  test("ofis panelinden ve mükellef panelinden Yardım açılır", async ({ page, browser }) => {
    await page.goto("/yardim");
    await expect(page.getByRole("heading", { name: "Yardım" })).toBeVisible();
    for (const title of ["Ofis yöneticisi kılavuzu", "Ofis çalışanı kılavuzu", "Mükellef kılavuzu", "Masaüstü ajanı kılavuzu"]) {
      await expect(page.getByRole("link", { name: title }).first()).toBeVisible();
    }
    await page.getByRole("link", { name: "Masaüstü ajanı kılavuzu" }).first().click();
    await expect(page).toHaveURL(/\/yardim\/masaustu$/);
    const img = page.locator("article img").first();
    await expect(img).toBeVisible();
    expect(await img.evaluate((el: HTMLImageElement) => el.naturalWidth)).toBeGreaterThan(0);

    await uiLogin(page, DEMO.owner);
    await page.goto(officePath());
    // Ofis sahibi kendi kılavuzuna gider (#522).
    await page.getByRole("link", { name: "Yardım" }).first().click();
    await expect(page).toHaveURL(/\/yardim\/ofis-yoneticisi$/);
    await expect(page.getByRole("heading", { level: 1, name: "Ofis yöneticisi kılavuzu" })).toBeVisible();

    // Mükellef ayrı bir tarayıcı bağlamında: çerez silmek oturumu her zaman
    // hemen kapatmıyordu, giriş formu bazen hiç görünmüyordu.
    const tp = await (await browser.newContext()).newPage();
    try {
      await uiLogin(tp, DEMO.taxpayerUser);
      await tp.waitForURL(/\/m\/[0-9a-f-]{36}/);
      await tp.getByRole("link", { name: "Yardım" }).first().click();
      await expect(tp).toHaveURL(/\/yardim\/mukellef$/);
      await expect(tp.getByRole("heading", { name: "Mükellef kılavuzu" })).toBeVisible();
    } finally {
      await tp.context().close();
    }
  });
});

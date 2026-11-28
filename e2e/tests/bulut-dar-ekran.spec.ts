import { expect, test, type Page } from "@playwright/test";

import { Api, DEMO, engagementOf, officePath, uiLogin } from "./lib";

/**
 * #576 Dar ekran (375 px, #780) ve açık temada ofis ekranları taranır:
 * - sayfa yana kaymaz (belge genişliği ekranı aşmaz),
 * - ham kod (BNK_UNDOCUMENTED_PAYMENT gibi), çevrilmemiş anahtar
 *   (office.bank…), ham yer tutucu ({{…}}), "undefined" / "NaN" /
 *   "[object Object]" ve uzun ham ondalık (0.41000000) görünmez.
 *
 * Yalnızca okur; demo verisini değiştirmez. Her ekran yumuşak denetlenir:
 * bir ekrandaki hata öbürlerinin taranmasını durdurmaz, CI hepsini tek
 * koşuda listeler.
 */
const VIEWPORT = { width: 375, height: 812 };

const OFFICE = [
  "",
  "/taxpayers",
  "/command",
  "/warnings",
  "/review",
  "/journals",
  "/transfers",
  "/account-plan",
  "/tax-returns",
  "/taxpayer-requests",
  "/settings",
];

const ENGAGEMENT = ["", "/bank", "/warnings", "/documents", "/journals", "/review", "/expenses"];

const RAW: { name: string; re: RegExp }[] = [
  { name: "ham kod", re: /\b[A-Z][A-Z0-9]+_[A-Z0-9_]{3,}\b/ },
  { name: "çevrilmemiş anahtar", re: /\b(office|common|taxpayer|nav|errors)\.[a-z_]+\.[a-z_.]+\b/ },
  { name: "ham yer tutucu", re: /\{\{\s*\w+\s*\}\}/ },
  { name: "tanımsız değer", re: /\b(undefined|NaN)\b|\[object Object\]/ },
  { name: "ham ondalık", re: /\b\d+[.,]\d{7,}\b/ },
];

async function settle(page: Page) {
  await page.waitForLoadState("networkidle");
  await expect(page.locator("main").first()).toBeVisible();
  // İskelet (yükleniyor) yer tutucuları gitsin.
  await expect
    .poll(async () => page.locator('[data-slot="skeleton"], .animate-pulse').count(), { timeout: 15_000 })
    .toBe(0)
    .catch(() => undefined);
}

/** Ekranın sağından taşan öğeler (ilk 5): hata iletisinde neyin taştığı görünsün. */
async function overflow(page: Page) {
  return page.evaluate(() => {
    const width = window.innerWidth;
    const extra = document.documentElement.scrollWidth - width;
    if (extra <= 1) return { extra, culprits: [] as string[] };
    const culprits: string[] = [];
    for (const el of Array.from(document.body.querySelectorAll<HTMLElement>("*"))) {
      const r = el.getBoundingClientRect();
      if (r.width === 0 || r.right <= width + 1) continue;
      // Kendi içinde kayan kabın (tablo sarmalayıcısı) içi sayılmaz.
      let p = el.parentElement;
      let scrolled = false;
      while (p && p !== document.body) {
        const ox = getComputedStyle(p).overflowX;
        if (ox === "auto" || ox === "scroll" || ox === "hidden" || ox === "clip") {
          scrolled = true;
          break;
        }
        p = p.parentElement;
      }
      if (scrolled) continue;
      const id = el.getAttribute("data-testid") ?? el.id ?? "";
      const text = (el.innerText ?? "").trim().slice(0, 40);
      culprits.push(`${el.tagName.toLowerCase()}${id ? `#${id}` : ""} → ${Math.round(r.right)}px "${text}"`);
      if (culprits.length >= 5) break;
    }
    return { extra, culprits };
  });
}

async function scan(page: Page, path: string) {
  await test.step(path, async () => {
    await page.goto(path);
    await expect(page.locator("html")).toHaveAttribute("data-theme", "m3-light");
    await settle(page);

    const o = await overflow(page);
    expect.soft(o.extra, `${path}: sayfa ${o.extra}px yana taşıyor\n${o.culprits.join("\n")}`).toBeLessThanOrEqual(1);

    const text = await page.locator("body").innerText();
    for (const { name, re } of RAW) {
      const hit = text.match(re);
      expect.soft(hit?.[0] ?? null, `${path}: ${name} görünüyor`).toBeNull();
    }
  });
}

test.describe("#576 dar ekran ve açık tema taraması", () => {
  test.use({ viewport: VIEWPORT, colorScheme: "light", reducedMotion: "reduce" });

  let atlas = "";

  test.beforeAll(async () => {
    const api = await Api.login(DEMO.owner);
    try {
      atlas = await engagementOf(api, DEMO.atlas.title);
    } finally {
      await api.dispose();
    }
  });

  test.beforeEach(async ({ page }) => {
    await page.addInitScript(() => {
      localStorage.setItem("app.palette", "m3");
      localStorage.setItem("app.theme", "light");
    });
  });

  test("ofis ekranları 375 px'te taşmaz, ham metin göstermez", async ({ page }) => {
    test.setTimeout(300_000);
    await uiLogin(page, DEMO.owner);
    for (const p of OFFICE) await scan(page, officePath(p));
  });

  test("mükellef ekranları 375 px'te taşmaz, ham metin göstermez", async ({ page }) => {
    test.setTimeout(300_000);
    await uiLogin(page, DEMO.owner);
    for (const p of ENGAGEMENT) await scan(page, officePath(`/engagements/${atlas}${p}`));
  });

  // #780: mükellef portalının ana sayfası (geçmiş veri paylaşımı kartındaki
  // uzun açık rıza bağlantısı 375 px'te sayfayı yana kaydırıyordu).
  test("mükellef portalı ana sayfası 375 px'te taşmaz", async ({ page }) => {
    test.setTimeout(120_000);
    await uiLogin(page, DEMO.taxpayerUser);
    await scan(page, new URL(page.url()).pathname);
  });
});

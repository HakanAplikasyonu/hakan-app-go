import { expect, test, type Page } from "@playwright/test";

import { DEMO, officePath, uiLogin } from "./lib";

/**
 * Görsel regresyon (#332): ana ekranlar dört temada (m3/n3 × açık/koyu) ve
 * üç kırılımda (≥1280, 768–1279, <768) temel görüntüyle karşılaştırılır.
 *
 * Temiz demo verisinde koşar (CI'da a11y'den hemen sonra, akışlar veriyi
 * değiştirmeden önce). Yalnızca okur.
 *
 * Değişen içerik maskelenir: `time` öğeleri, `[data-visual-mask]` ve
 * tarihe bağlı metinler (DYNAMIC_TEXT: dönem "Eki 26", "2026-10",
 * "2026-11-30", "16.11.2026", "14 gün kaldı"). Demo verisi bugüne göre
 * üretildiği için bunlar her gün / ay değişir. Yeni bir ekran başka oynak
 * içerik gösteriyorsa öğesine `data-visual-mask` ekleyin.
 *
 * Temel görüntüleri güncellemek (tasarım bilerek değiştiyse):
 *   cd e2e && npx playwright test --grep @visual --update-snapshots
 * CI'daki farklar `e2e-report` artefaktında (beklenen / gerçek / fark).
 */
const THEMES = ["m3-light", "m3-dark", "n3-light", "n3-dark"] as const;
type Theme = (typeof THEMES)[number];

const VIEWPORTS = [
  { name: "masaustu", width: 1440, height: 900 },
  { name: "tablet", width: 1024, height: 768 },
  { name: "telefon", width: 390, height: 844 },
] as const;

const DYNAMIC_TEXT = [
  /(?<!\p{L})(Oca|Şub|Mar|Nis|May|Haz|Tem|Ağu|Eyl|Eki|Kas|Ara) \d{2}(?!\d)/u,
  /\b20\d{2}-\d{2}(-\d{2})?\b/,
  /\b\d{2}\.\d{2}\.20\d{2}\b/,
  /gün (kaldı|geçti)/,
];

/** Tema seçimi: providers/theme-provider (app.theme + app.palette). */
async function applyTheme(page: Page, theme: Theme) {
  const [palette, mode] = theme.split("-");
  await page.evaluate(
    ([p, m]) => {
      localStorage.setItem("app.palette", p);
      localStorage.setItem("app.theme", m);
    },
    [palette, mode],
  );
}

async function settle(page: Page) {
  await page.waitForLoadState("networkidle");
  // Web yazı tipleri ve geç gelen sorgular otursun.
  await page.evaluate(() => document.fonts.ready);
  await page.waitForTimeout(300);
}

async function snap(page: Page, path: string, name: string) {
  for (const theme of THEMES) {
    for (const vp of VIEWPORTS) {
      await test.step(`${name} · ${theme} · ${vp.name}`, async () => {
        await page.setViewportSize({ width: vp.width, height: vp.height });
        await applyTheme(page, theme);
        await page.goto(path);
        await expect(page.locator("html")).toHaveAttribute("data-theme", theme);
        await settle(page);
        // Yumuşak: bir fark sonraki çekimleri durdurmaz; CI bütün farkları
        // (ve -actual görüntülerini) tek koşuda toplar.
        await expect.soft(page).toHaveScreenshot(`${name}-${theme}-${vp.name}.png`, {
          animations: "disabled",
          caret: "hide",
          mask: [
            page.locator("time"),
            page.locator("[data-visual-mask]"),
            ...DYNAMIC_TEXT.map((re) => page.getByText(re)),
          ],
          maxDiffPixelRatio: 0.01,
        });
      });
    }
  }
}

test.describe("@visual görsel regresyon", () => {
  test.use({ reducedMotion: "reduce" });

  test("misafir: giriş", async ({ page }) => {
    await page.goto("/login");
    await snap(page, "/login", "giris");
  });

  test("ofis: genel bakış ve inceleme kuyruğu", async ({ page }) => {
    await uiLogin(page, DEMO.owner);
    await snap(page, officePath(), "ofis-genel-bakis");
    await snap(page, officePath("/review"), "ofis-inceleme");
  });

  test("mükellef: portal ana sayfa", async ({ page }) => {
    await uiLogin(page, DEMO.taxpayerUser);
    await page.waitForURL(/\/m\/[0-9a-f-]{36}/);
    await snap(page, new URL(page.url()).pathname, "mukellef-ana-sayfa");
  });
});

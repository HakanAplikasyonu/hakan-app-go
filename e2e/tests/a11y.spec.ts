import AxeBuilder from "@axe-core/playwright";
import { expect, test, type Page } from "@playwright/test";

import { Api, DEMO, engagementOf, officePath, uiLogin } from "./lib";

/**
 * Erişilebilirlik denetimi (#194): ana sayfalarda axe-core (WCAG 2.1 A/AA).
 * Kritik ve ciddi (critical / serious) ihlal testi kırar; orta ve küçük
 * ihlaller test ekine (annotation) yazılır. Yalnızca okur: diğer dosyalarla
 * paralel koşar.
 *
 * Klavye: komut paleti Ctrl+K ile açılır, odak aramada, Esc ile kapanır.
 */
const BLOCKING = new Set(["critical", "serious"]);

async function scan(page: Page, name: string) {
  // İçerik yüklensin: yükleniyor iskeletleri ve geçici durumlar denetlenmez.
  await page.waitForLoadState("networkidle");
  const result = await new AxeBuilder({ page })
    .withTags(["wcag2a", "wcag2aa", "wcag21a", "wcag21aa"])
    // Realtime hata ayıklama paneli yalnızca `next dev`'de çizilir (üretimde yok).
    .exclude('[aria-label^="Realtime debug"]')
    .exclude('[title^="Realtime ·"]')
    .analyze();
  const blocking = result.violations.filter((v) => BLOCKING.has(v.impact ?? ""));
  const minor = result.violations.filter((v) => !BLOCKING.has(v.impact ?? ""));
  for (const v of minor) {
    test.info().annotations.push({ type: `a11y:${v.impact}`, description: `${name}: ${v.id} (${v.nodes.length})` });
  }
  const report = blocking
    .map(
      (v) =>
        `${v.impact} ${v.id}: ${v.help}\n` +
        v.nodes
          .slice(0, 5)
          .map((n) => `    ${n.target.join(" ")} — ${n.failureSummary?.split("\n").slice(1, 2).join("").trim()}`)
          .join("\n"),
    )
    .join("\n");
  expect(blocking, `${name}\n${report}`).toEqual([]);
}

test.describe("@a11y misafir sayfaları", () => {
  for (const path of ["/login", "/platform/register", "/forgot-password", "/yardim", "/yardim/ofis-yoneticisi"]) {
    test(path, async ({ page }) => {
      await page.goto(path);
      await scan(page, path);
    });
  }
});

test.describe("@a11y ofis paneli", () => {
  let atlas = "";
  test.beforeAll(async () => {
    const api = await Api.login(DEMO.owner);
    atlas = await engagementOf(api, DEMO.atlas.title);
    await api.dispose();
  });

  test("ana ekranlar", async ({ page }) => {
    await uiLogin(page, DEMO.owner);
    const screens = [
      "",
      "/copilot",
      "/command",
      "/taxpayers",
      "/taxpayer-requests",
      `/invoices?taxpayer=${atlas}`,
      "/review",
      "/warnings",
      `/upload?taxpayer=${atlas}`,
      `/journals?taxpayer=${atlas}`,
      `/engagements/${atlas}/expenses`,
      `/engagements/${atlas}/z-reports`,
      "/agents",
      "/account-plan",
      "/members",
      "/roles",
      "/settings",
    ];
    for (const s of screens) {
      await test.step(s || "/", async () => {
        await page.goto(officePath(s));
        await scan(page, officePath(s));
      });
    }
  });

  test("klavye: komut paleti Ctrl+K ile açılır ve Esc ile kapanır", async ({ page }) => {
    await uiLogin(page, DEMO.owner);
    await page.goto(officePath());
    await page.waitForLoadState("networkidle");
    await page.keyboard.press("Control+k");
    const dialog = page.getByRole("dialog");
    await expect(dialog).toBeVisible();
    await expect(dialog.getByRole("combobox").or(dialog.getByRole("textbox")).first()).toBeFocused();
    await scan(page, "komut paleti");
    await page.keyboard.press("Escape");
    await expect(dialog).toBeHidden();
  });

  // #285: çevrimdışı rozeti ve genel bakış kartı yalnızca çevrimdışı ajan
  // varken görünür; sunucu bunu 30 dk sessizlikten sonra yapar, liste taklit edilir.
  test("çevrimdışı masaüstü ajanı: genel bakış kartı ve ajan listesi", async ({ page }) => {
    const at = new Date(Date.now() - 3_600_000).toISOString();
    await page.route("**/api/v1/tenant/agents", (route) =>
      route.fulfill({
        json: {
          success: true,
          data: {
            items: [
              {
                uuid: "00000000-0000-4000-8000-000000000285",
                machine_id: "a11y-agent-0001",
                name: "A11y-PC",
                platform: "windows",
                version: "0.1.0",
                status: "offline",
                last_seen_at: at,
                offline_since: at,
                heartbeat: {},
                created_at: at,
                last_error: { code: "AGENT_OFFLINE", at },
                heartbeat_seconds: 300,
              },
            ],
          },
          meta: { request_id: "a11y" },
        },
      }),
    );
    await uiLogin(page, DEMO.owner);
    for (const p of ["", "/agents"]) {
      await page.goto(officePath(p));
      await expect(page.getByText("A11y-PC").first()).toBeVisible();
      await scan(page, `${officePath(p)} (çevrimdışı ajan)`);
    }
  });
});

test.describe("@a11y mükellef paneli", () => {
  test("ana ekranlar", async ({ page }) => {
    await uiLogin(page, DEMO.taxpayerUser);
    await page.waitForURL(/\/m\/[0-9a-f-]{36}/);
    const base = new URL(page.url()).pathname;
    for (const s of ["", "/invoices", "/receipts", "/uploads"]) {
      await test.step(s || "/", async () => {
        await page.goto(base + s);
        await scan(page, base + s);
      });
    }
  });
});

import { expect, test } from "@playwright/test";

import { scan } from "./a11y";
import { Api, DEMO, engagementOf, letterToken, officePath, uiLogin } from "./lib";

/**
 * Erişilebilirlik denetimi (#194, #455): ana sayfalarda axe-core (WCAG 2.1
 * A/AA, `./a11y`). Kritik ve ciddi (critical / serious) ihlal testi kırar;
 * orta ve küçük ihlaller test ekine (annotation) yazılır. Yalnızca okur:
 * diğer dosyalarla paralel koşar.
 *
 * Klavye: komut paleti Ctrl+K ile açılır, odak aramada, Esc ile kapanır.
 * Durumla açılan diyaloglarda odak içeri taşınır, Esc ile açan düğmeye döner.
 */
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
      // Ofis beyan dönemi (#575).
      "/tax-returns",
      "/taxpayer-requests",
      `/invoices?taxpayer=${atlas}`,
      "/review",
      // Düzenlenebilir ızgara (#394).
      "/review?view=grid",
      "/warnings",
      // Toplu rapor gönderimi (#386), WhatsApp eşleşmeleri (#388).
      "/reports",
      "/whatsapp",
      `/upload?taxpayer=${atlas}`,
      `/journals?taxpayer=${atlas}`,
      // Ofis geneli programa aktarımlar (#574).
      "/transfers",
      // Co-pilot çubuğu mükellef ekranlarında (#392).
      `/engagements/${atlas}`,
      `/engagements/${atlas}/payroll`,
      `/engagements/${atlas}/easytrade`,
      `/engagements/${atlas}/documents`,
      `/engagements/${atlas}/petitions`,
      `/engagements/${atlas}/bank`,
      `/engagements/${atlas}/report`,
      `/engagements/${atlas}/expenses`,
      `/engagements/${atlas}/z-reports`,
      `/engagements/${atlas}/reconciliation`,
      `/engagements/${atlas}/reconciliation?view=counterparties`,
      `/engagements/${atlas}/reconciliation?view=babs`,
      `/engagements/${atlas}/reconciliation?view=letters`,
      // Beyannameler: KDV1 (taslak, kontrol listesi), MUHSGK ve Ba/Bs (#390).
      `/engagements/${atlas}/tax-returns`,
      `/engagements/${atlas}/tax-returns?kind=muhsgk`,
      `/engagements/${atlas}/tax-returns?kind=babs`,
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

  // #455: durumla (Trigger'sız) açılan diyaloglarda odak body'ye düşüyordu.
  test("klavye: diyalog Enter ile açılır, odak içeride, Esc ile açan düğmeye döner", async ({ page }) => {
    await uiLogin(page, DEMO.owner);
    const cases = [
      { path: `/engagements/${atlas}/expenses`, button: "Kategoriler" },
      { path: `/engagements/${atlas}/report`, button: "E-postayla gönder" },
      { path: `/engagements/${atlas}/reconciliation`, button: "Ayrıntı" },
    ];
    for (const c of cases) {
      await test.step(`${c.path} · ${c.button}`, async () => {
        await page.goto(officePath(c.path));
        const opener = page.getByRole("main").getByRole("button", { name: c.button, exact: true }).first();
        await expect(opener).toBeVisible({ timeout: 30_000 });
        await opener.focus();
        await page.keyboard.press("Enter");
        const dialog = page.getByRole("dialog");
        await expect(dialog).toBeVisible();
        await expect
          .poll(() => dialog.evaluate((d) => d.contains(document.activeElement)), { message: "odak diyalogda" })
          .toBe(true);
        await scan(page, `${c.path} · ${c.button} diyaloğu`, { include: '[role="dialog"]' });
        await page.keyboard.press("Escape");
        await expect(dialog).toBeHidden();
        await expect(opener).toBeFocused();
      });
    }
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
    for (const s of ["", "/invoices", "/receipts", "/uploads", "/payroll", "/report", "/requests"]) {
      await test.step(s || "/", async () => {
        await page.goto(base + s);
        await scan(page, base + s);
      });
    }
  });
});

// Karşı firmanın mutabakat yanıt sayfası (#389): oturumsuz, mektup bağlantısıyla.
test.describe("@a11y mutabakat yanıt sayfası", () => {
  test("geçerli ve geçersiz bağlantı", async ({ page }) => {
    const api = await Api.login(DEMO.owner);
    const atlas = await engagementOf(api, DEMO.atlas.title);
    const letters = await api.json<{ items: { uuid: string; status: string }[] }>(
      await api.get(`/v1/tenant/engagements/${atlas}/reconciliation/letters`),
    );
    await api.dispose();
    // Demo verisi: yanıt bekleyen bir mektup. Demo tohumu taslak bırakır
    // (gönderim taraması e2e'de yok, #461); bağlantı taslakta da açılır. Önce
    // taslak seçilir: akış testinin (mutabakat.spec.ts) kuyruğa aldığı ve
    // yanıtladığı mektupla yarışılmaz.
    const open =
      letters.items.find((l) => l.status === "draft") ??
      letters.items.find((l) => l.status === "queued" || l.status === "sent");
    expect(open, "yanıt bekleyen demo mektubu").toBeTruthy();
    await page.goto(`/mutabakat/${letterToken(open!.uuid)}`);
    await expect(page.getByRole("heading", { name: "Cari hesap mutabakatı" })).toBeVisible();
    await scan(page, "/mutabakat/{belirteç}");
    await page.goto("/mutabakat/gecersiz-belirtec-0000000000000000");
    await scan(page, "/mutabakat/{geçersiz}");
  });
});

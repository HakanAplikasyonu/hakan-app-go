import { expect, test } from "@playwright/test";

import { Api, DEMO, engagementOf, officePath, uiLogin } from "./lib";

/**
 * Rol bazlı erişim reddi (#181). Yalnızca okur; diğer dosyalarla paralel koşar.
 *
 * Demo ofisinde stajyer (Personel) yalnızca Selin'e atanmış, Atlas'a değil;
 * mükellef kullanıcısı ofis üyesi değil.
 */
test.describe("giriş yapmamış kullanıcı", () => {
  for (const path of [officePath("/review"), "/m"]) {
    test(`${path} → giriş sayfası`, async ({ page }) => {
      await page.goto(path);
      await page.waitForURL(/\/login\?next=/);
      await expect(page.locator('input[type="password"]').first()).toBeVisible();
    });
  }
});

test.describe("stajyer (Personel)", () => {
  test.beforeEach(async ({ page }) => {
    await uiLogin(page, DEMO.intern);
  });

  test("roller sayfası kapalı, üye daveti ve ayar kaydı yok", async ({ page }) => {
    await page.goto(officePath("/roles"));
    await expect(page.getByText("Bu sayfa için yetkin yok")).toBeVisible();

    await page.goto(officePath("/members"));
    await expect(page.getByText(DEMO.owner)).toBeVisible();
    await expect(page.getByRole("button", { name: "Davet gönder" })).toHaveCount(0);

    await page.goto(officePath("/settings"));
    await expect(page.getByText("Organizasyon ayarları").first()).toBeVisible();
    await expect(page.getByRole("button", { name: "Kaydet" })).toHaveCount(0);
  });

  test("platform paneli kapalı", async ({ page }) => {
    await page.goto("/platform");
    await expect(page.getByText("Bu sayfa için yetkin yok").first()).toBeVisible();
  });

  test("atanmadığı mükellef ekranlarda yok", async ({ page }) => {
    for (const screen of ["/journals", "/upload", "/invoices"]) {
      await page.goto(officePath(screen));
      const picker = page.locator(`select:has(option:text-matches("^${DEMO.selin.title}"))`).first();
      await expect(picker).toBeVisible();
      await expect(picker.locator("option", { hasText: DEMO.atlas.title })).toHaveCount(0);
    }
  });

  test("atanmadığı mükellefe API'den yazamıyor", async () => {
    const owner = await Api.login(DEMO.owner);
    const atlas = await engagementOf(owner, DEMO.atlas.title);
    await owner.dispose();

    const intern = await Api.login(DEMO.intern);
    try {
      for (const [method, path] of [
        ["GET", `/v1/tenant/engagements/${atlas}/journal-drafts`],
        ["POST", `/v1/tenant/engagements/${atlas}/journal-exports`],
        ["POST", `/v1/tenant/engagements/${atlas}/periods/2026-09/lock`],
        ["POST", `/v1/tenant/engagements/${atlas}/cash-boxes`],
      ] as const) {
        const res = method === "GET" ? await intern.get(path) : await intern.post(path, {});
        expect(res.status(), `${method} ${path}`).toBe(403);
        // Rolün izni yoksa önce izin (FORBIDDEN), varsa atama (NO_ENGAGEMENT_ACCESS) reddeder.
        expect(["NO_ENGAGEMENT_ACCESS", "FORBIDDEN"], `${method} ${path}`).toContain(
          (await res.json()).error.code,
        );
      }
    } finally {
      await intern.dispose();
    }
  });
});

test.describe("mükellef kullanıcısı", () => {
  test("ofis panelinde ve platformda erişim yok", async ({ page }) => {
    await uiLogin(page, DEMO.taxpayerUser);
    for (const screen of ["", "/invoices"]) {
      await page.goto(officePath(screen));
      await expect(page.getByText("Bu organizasyona erişiminiz yok")).toBeVisible();
    }
    await page.goto("/platform");
    await expect(page.getByText("Bu sayfa için yetkin yok").first()).toBeVisible();
  });
});

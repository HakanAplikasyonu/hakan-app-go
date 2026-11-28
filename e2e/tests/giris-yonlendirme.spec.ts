import { expect, test, type Browser, type Page } from "@playwright/test";

import { DEMO, mailToken, officePath, registerWithToken, uiLogin, uniqueEmail } from "./lib";

/**
 * Giriş, çıkış ve profil yönlendirmeleri (#890 A+B+C):
 * - ofis kullanıcısı girişte doğrudan ofisine, `/` adresinde de ofisine gider;
 * - profil ofis kenar çubuğuyla açılır, eski `/profile` oraya yönlenir;
 * - çıkış `/login` açar (platform girişi değil);
 * - iki ofisli kullanıcı ofis değiştirip çıkarsa tekrar girişte aynı ofise döner.
 */

async function logout(page: Page) {
  await page.getByRole("banner").getByRole("button", { name: /^Profil/ }).click();
  await page.getByRole("menuitem", { name: "Çıkış yap" }).click();
  await page.waitForURL((u) => u.pathname === "/login");
}

test.describe("tek ofisli kullanıcı", () => {
  test.beforeEach(async ({ page }) => {
    await page.setViewportSize({ width: 1440, height: 900 });
  });

  test("giriş → ofis; / → ofis; profil ofis kabuğunda; çıkış → /login", async ({ page }) => {
    await uiLogin(page, DEMO.owner);
    await expect(page).toHaveURL(new RegExp(`${officePath()}$`));

    // `/` CMS ana sayfası değil: ofise yönlenir.
    await page.goto("/");
    await page.waitForURL((u) => u.pathname === officePath());

    // Menüdeki Profil ofis kabuğundaki profile gider.
    await page.getByRole("banner").getByRole("button", { name: /^Profil/ }).click();
    await page.getByRole("menuitem", { name: "Profil" }).click();
    await page.waitForURL((u) => u.pathname === officePath("/profil"));
    await expect(page.getByLabel("Ofis menüsü").first()).toBeVisible();
    await expect(page.getByRole("heading", { level: 1, name: "Profil" })).toBeVisible();

    // Eski adres kendi kabuğundaki profile yönlenir.
    await page.goto("/profile");
    await page.waitForURL((u) => u.pathname === officePath("/profil"));
    await expect(page.getByLabel("Ofis menüsü").first()).toBeVisible();

    // `?next=/` ofis kullanıcısı için yok sayılır.
    await logout(page);
    await expect(page).toHaveURL(/\/login$/);
    await page.goto("/login?next=%2F");
    await page.locator('input[name="email"], input[type="email"]').first().fill(DEMO.owner);
    await page.locator('input[type="password"]').first().fill(DEMO.password);
    await page.locator('button[type="submit"]').first().click();
    await page.waitForURL((u) => u.pathname === officePath());
  });

  test("eski /platform/login adresi açılır", async ({ page }) => {
    await page.goto("/platform/login");
    await expect(page.locator('input[type="password"]').first()).toBeVisible();
  });
});

test.describe("iki ofisli kullanıcı", () => {
  test.describe.configure({ mode: "serial" });

  const run = `${Date.now()}`.slice(-7);
  const slugA = `e2e-giris-a-${run}`;
  const slugB = `e2e-giris-b-${run}`;
  const email = uniqueEmail("ikiofis");
  const password = "E2e-Parola-123!";

  /** Platform yöneticisi ofisi açar ve `email`'i sahip olarak davet eder. */
  async function openOfficeAndInvite(browser: Browser, slug: string) {
    const admin = await (await browser.newContext()).newPage();
    try {
      await uiLogin(admin, DEMO.admin, DEMO.adminPassword);
      await admin.goto("/platform/organizations/create");
      await admin.locator('input[name="name"]').fill(`E2E Giriş ${slug}`);
      await admin.locator('input[name="slug"]').fill(slug);
      await admin.getByRole("button", { name: "Organizasyon oluştur" }).click();
      await admin.waitForURL((u) => /\/platform\/organizations\/[0-9a-f-]{36}/.test(u.pathname));
      await admin.goto(`/t/${slug}/members`);
      await admin.locator('input[name="email"]').fill(email);
      await admin.getByRole("combobox", { name: "Rol" }).click();
      await admin.getByRole("option", { name: "Sahip", exact: true }).click();
      await admin.getByRole("button", { name: "Davet gönder" }).click();
      await expect(admin.getByText(email).first()).toBeVisible();
    } finally {
      await admin.context().close();
    }
  }

  /** En yeni davet belirteci; `not` verilirse ondan farklı olanı bekler. */
  async function inviteToken(not?: string): Promise<string> {
    for (let i = 0; i < 30; i++) {
      const token = await mailToken(email, "/invite/");
      if (token !== not) return token;
      await new Promise((r) => setTimeout(r, 1000));
    }
    throw new Error(`yeni davet e-postası gelmedi: ${email}`);
  }

  test("ofis değiştir → çıkış → giriş → aynı ofis", async ({ browser }) => {
    test.setTimeout(240_000);
    await openOfficeAndInvite(browser, slugA);
    const tokenA = await inviteToken();
    await openOfficeAndInvite(browser, slugB);
    const tokenB = await inviteToken(tokenA);

    const page = await (await browser.newContext({ viewport: { width: 1440, height: 900 } })).newPage();
    try {
      await registerWithToken(page, tokenA, `/invite/${tokenA}`, { name: "İki", surname: "Ofis", password });
      await page.getByRole("button", { name: "Daveti kabul et" }).click();
      await page.waitForURL((u) => u.pathname.startsWith(`/t/${slugA}`));
      await page.goto(`/invite/${tokenB}`);
      await page.getByRole("button", { name: "Daveti kabul et" }).click();
      await page.waitForURL((u) => u.pathname.startsWith(`/t/${slugB}`));

      // A'yı aç, menüden B'ye geç, çık.
      await page.goto(`/t/${slugA}`);
      await page.getByRole("banner").getByRole("button", { name: /^Profil/ }).click();
      await page.getByRole("menuitem", { name: new RegExp(slugB) }).click();
      await page.waitForURL((u) => u.pathname === `/t/${slugB}`);
      await logout(page);

      // Tekrar giriş: seçici değil, doğrudan son ofis.
      await uiLogin(page, email, password);
      await page.waitForURL((u) => u.pathname === `/t/${slugB}`);
    } finally {
      await page.context().close();
    }
  });
});

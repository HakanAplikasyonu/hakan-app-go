import { expect, test, type Page } from "@playwright/test";

import { scan } from "./a11y";
import {
  Api,
  consentTokenFor,
  DEMO,
  deriveUbl,
  engagementOf,
  officePath,
  registerWithToken,
  uiLogin,
  uniqueEmail,
  uniqueVkn,
  waitForUploads,
} from "./lib";

/**
 * Mükellef detayında sekme yapısı (#468): her sekme ve sekme içinden açılan
 * detay mükellef rotasında kalır; başlık, sekme çubuğu ve dönem şeridi
 * kaybolmaz; detaydan geri dönüş listenin süzgecini korur.
 *
 * Veri: demo ofisine her koşuda yeni bir mükellef eklenir ve demo UBL'lerinden
 * türetilen üç fatura yüklenir (Atlas'ın faturası smoke testinin sayımına
 * girdiği için ona dokunulmaz). Sekme gezintisi Atlas Gıda'da, yalnızca okur.
 */
test.describe.configure({ mode: "serial" });

const run = `${Date.now()}`.slice(-6);
const taxpayer = { vkn: uniqueVkn(), title: `Sekme Ticaret ${run} A.Ş.` };
const UBL = ["atlas-01-temel-satis.xml", "atlas-02-ticari-satis.xml", "atlas-05-iade-alis.xml"];

test.describe("mükellef sekmeleri", () => {
  let atlas = "";
  let engagement = "";

  test.beforeAll(async ({ browser }) => {
    test.setTimeout(180_000);
    const api = await Api.login(DEMO.owner);
    try {
      atlas = await engagementOf(api, DEMO.atlas.title);

      // Yeni mükellef: onay iste → mükellef kaydolur ve onaylar.
      const contact = uniqueEmail("sekme");
      const office = await (await browser.newContext()).newPage();
      try {
        await uiLogin(office, DEMO.owner);
        await office.goto(officePath("/taxpayer-requests"));
        await office.locator('input[name="tax_id"]').fill(taxpayer.vkn);
        await office.locator('input[name="title"]').fill(taxpayer.title);
        await office.locator('input[name="contact_email"]').fill(contact);
        await office.getByRole("button", { name: "Onay iste" }).click();
        await expect(office.getByText(contact).first()).toBeVisible();
      } finally {
        await office.context().close();
      }
      const token = await consentTokenFor(contact);
      const tp = await (await browser.newContext()).newPage();
      try {
        await registerWithToken(tp, token, `/consent/${token}`, {
          name: "Sekme",
          surname: "Mükellef",
          password: "E2e-Parola-123!",
        });
        await expect(tp.getByRole("checkbox").first()).toBeVisible();
        for (const box of await tp.getByRole("checkbox").all()) await box.check();
        await tp.getByRole("button", { name: "Onayla" }).click();
        await tp.waitForURL(/\/m\/[0-9a-f-]{36}/);
      } finally {
        await tp.context().close();
      }
      engagement = await engagementOf(api, taxpayer.title);

      for (const f of UBL) {
        const res = await api.upload(`/v1/tenant/engagements/${engagement}/ubl-uploads`, deriveUbl(f, taxpayer.vkn, taxpayer.title));
        expect(res.status(), `${f}: ${await res.text()}`).toBeLessThan(300);
      }
      await waitForUploads(api, engagement, UBL.length);
    } finally {
      await api.dispose();
    }
  });

  const tabs = (page: Page) => page.getByRole("navigation", { name: "Mükellef bölümleri" });

  async function expectShell(page: Page, title: string = DEMO.atlas.title) {
    await expect(page.getByRole("heading", { level: 1, name: title })).toBeVisible();
    await expect(tabs(page)).toBeVisible();
    await expect(page.getByRole("heading", { name: "Dönem şeridi" })).toBeVisible();
  }

  test("Faturalar sekmesi → fatura detayı → geri; sekme çubuğu kaybolmaz", async ({ page }) => {
    await uiLogin(page, DEMO.owner);
    await page.goto(officePath(`/engagements/${engagement}`));
    await expectShell(page, taxpayer.title);

    await tabs(page).getByRole("link", { name: "Faturalar" }).click();
    await page.waitForURL((u) => u.pathname.endsWith(`/engagements/${engagement}/invoices`));
    await expectShell(page, taxpayer.title);
    await expect(tabs(page).getByRole("link", { name: "Faturalar" })).toHaveAttribute("aria-current", "page");
    // Mükellef sabit: seçici yok, URL'de ?taxpayer= yok.
    expect(new URL(page.url()).searchParams.get("taxpayer")).toBeNull();

    await expect(page.locator('a[href*="/invoices/"]')).toHaveCount(UBL.length);

    // Süzgeç (giden faturalar) URL'de; detaya ve geri taşınır.
    await page.goto(officePath(`/engagements/${engagement}/invoices?direction=outgoing`));
    await expectShell(page, taxpayer.title);
    const first = page.locator('a[href*="/invoices/"]').first();
    await expect(first).toBeVisible();
    await first.click();
    await page.waitForURL((u) => /\/engagements\/[^/]+\/invoices\/[^/]+$/.test(u.pathname));
    await expectShell(page, taxpayer.title);
    await expect(tabs(page).getByRole("link", { name: "Faturalar" })).toHaveAttribute("aria-current", "true");
    expect(new URL(page.url()).searchParams.get("direction")).toBe("outgoing");

    await page.getByRole("link", { name: "Faturalara dön" }).first().click();
    await page.waitForURL((u) => u.pathname.endsWith(`/engagements/${engagement}/invoices`));
    expect(new URL(page.url()).searchParams.get("direction")).toBe("outgoing");
    await expectShell(page, taxpayer.title);

    // Tarayıcının geri tuşu da sekme içinde kalır.
    await page.goBack();
    await page.waitForURL((u) => /\/engagements\/[^/]+\/invoices\/[^/]+$/.test(u.pathname));
    await expectShell(page, taxpayer.title);
  });

  test("sekme içi bağlantılar mükellef rotasında kalır: uyarılar, yüklemeler, inceleme, fişler", async ({ page }) => {
    await uiLogin(page, DEMO.owner);
    const base = `/engagements/${engagement}`;

    // Genel bakıştaki "Açık uyarı" sayacı Uyarılar sekmesine gider.
    await page.goto(officePath(base));
    await expectShell(page, taxpayer.title);
    await page.getByRole("link", { name: /Açık uyarı/ }).click();
    await page.waitForURL((u) => u.pathname.endsWith(`${base}/warnings`));
    await expectShell(page, taxpayer.title);
    await expect(tabs(page).getByRole("link", { name: "Uyarılar" })).toHaveAttribute("aria-current", "page");
    // Mükellef sabit: mükellef seçicisi yok.
    await expect(page.getByRole("combobox", { name: "Mükellef" })).toHaveCount(0);

    // Yüklemeler: bu mükellefin işleri; "Faturalar" bağlantısı sekmede kalır.
    await tabs(page).getByRole("link", { name: "Yüklemeler" }).click();
    await page.waitForURL((u) => u.pathname.endsWith(`${base}/upload`));
    await expectShell(page, taxpayer.title);
    await expect(page.getByRole("combobox", { name: "Mükellef" })).toHaveCount(0);
    await page.locator("main p a", { hasText: "Faturalar" }).click();
    await page.waitForURL((u) => u.pathname.endsWith(`${base}/invoices`));
    await expectShell(page, taxpayer.title);

    // İnceleme: kuyruk bu mükellefe süzülü, sekme çubuğu yerinde.
    await tabs(page).getByRole("link", { name: "İnceleme" }).click();
    await page.waitForURL((u) => u.pathname.endsWith(`${base}/review`));
    await expectShell(page, taxpayer.title);
    await expect(page.getByRole("heading", { level: 2, name: "İnceleme kuyruğu" })).toBeVisible();
    await expect(page.getByRole("combobox", { name: "Mükellef" })).toHaveCount(0);

    // Fişler: yevmiye taslakları bu mükellefin.
    await tabs(page).getByRole("link", { name: "Fişler" }).click();
    await page.waitForURL((u) => u.pathname.endsWith(`${base}/journals`));
    await expectShell(page, taxpayer.title);
    await expect(page.getByRole("heading", { level: 2, name: "Yevmiye ve çıktı" })).toBeVisible();
    expect(new URL(page.url()).searchParams.get("taxpayer")).toBeNull();

    // Geri tuşu sekmeler arasında da çalışır.
    await page.goBack();
    await page.waitForURL((u) => u.pathname.endsWith(`${base}/review`));
    await expectShell(page, taxpayer.title);
  });

  test("her sekme aynı başlık ve sekme çubuğuyla açılır", async ({ page }) => {
    await uiLogin(page, DEMO.owner);
    await page.goto(officePath(`/engagements/${atlas}`));
    await expectShell(page);
    const names = await tabs(page).getByRole("link").allInnerTexts();
    expect(names.length).toBeGreaterThan(5);
    for (const name of names) {
      await test.step(name, async () => {
        await tabs(page).getByRole("link", { name, exact: true }).click();
        await expect(tabs(page).getByRole("link", { name, exact: true })).toHaveAttribute("aria-current", "page");
        await expect(page).toHaveURL(new RegExp(`/engagements/${atlas}`));
        await expectShell(page);
      });
    }
  });

  test("klavye: ok tuşları sekmeler arasında gezer; erişilebilirlik", async ({ page }) => {
    await uiLogin(page, DEMO.owner);
    await page.goto(officePath(`/engagements/${atlas}/invoices`));
    await expectShell(page);
    const links = tabs(page).getByRole("link");
    await links.first().focus();
    await page.keyboard.press("ArrowRight");
    await expect(links.nth(1)).toBeFocused();
    await page.keyboard.press("End");
    await expect(links.last()).toBeFocused();
    await page.keyboard.press("Home");
    await expect(links.first()).toBeFocused();

    await scan(page, "mükellef sekmeleri");
  });
});

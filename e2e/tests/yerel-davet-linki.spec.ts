import { expect, test, type Page } from "@playwright/test";

import {
  addConsentedTaxpayer,
  Api,
  DEMO,
  mailToken,
  officePath,
  uiLogin,
  uniqueEmail,
  uniqueVkn,
} from "./lib";

/**
 * Mükellef davet linki (Oğuz'un geri bildirimi: davet e-postası canlıda
 * gelmiyor). Davet yanıtı linki döner (invite_url); mükellef sayfasında
 * "Davet linkini kopyala" ("Kopyalandı" onayı) ve "WhatsApp ile paylaş"
 * (wa.me bağlantısı, link URL kodlu) çıkar. E-posta gönderilemezse ekran
 * "E-posta gönderilemedi, bağlantıyı kopyalayıp paylaşın" der; davet yine
 * oluşur. Bekleyen davette "Yeniden gönder" yeni link üretir.
 *
 * Veri: her koşuda yeni bir mükellef ve benzersiz davet adresleri. Ofis
 * geneli ayar değiştirilmez.
 */
test.describe.configure({ mode: "serial" });

const run = `${Date.now()}`.slice(-6);
const taxpayer = { vkn: uniqueVkn(), title: `Davet Linki ${run} A.Ş.` };

let api: Api;
let engagement = "";

test.beforeAll(async ({ browser }) => {
  test.setTimeout(240_000);
  api = await Api.login(DEMO.owner);
  engagement = await addConsentedTaxpayer(browser, api, taxpayer, "davetlinki");
});

test.afterAll(async () => {
  await api?.dispose();
});

async function openSection(page: Page) {
  await page.context().grantPermissions(["clipboard-read", "clipboard-write"]);
  await uiLogin(page, DEMO.owner);
  await page.goto(officePath(`/engagements/${engagement}`));
  const section = page.getByTestId("app-invite-section");
  await expect(section).toBeVisible({ timeout: 30_000 });
  return section;
}

async function invite(page: Page, email: string) {
  const section = page.getByTestId("app-invite-section");
  await section.locator("#app-invite-email").fill(email);
  await section.getByRole("button", { name: "Davet gönder" }).click();
}

test("davet linki kopyalanır, WhatsApp bağlantısı linki taşır", async ({ page }) => {
  const section = await openSection(page);
  const email = uniqueEmail("davetlinki");
  await invite(page, email);

  const panel = section.getByTestId("invite-link-panel");
  await expect(panel).toBeVisible();
  await expect(panel.getByTestId("invite-email-sent")).toBeVisible();

  // E-postadaki link ile ekrandaki link aynı.
  const token = await mailToken(email, "/invite/m/");
  const copy = panel.getByTestId("invite-copy");
  await expect(copy).toHaveText("Davet bağlantısını kopyala");
  await copy.click();
  await expect(copy).toHaveText("Kopyalandı");
  const copied = await page.evaluate(() => navigator.clipboard.readText());
  expect(copied).toMatch(new RegExp(`/invite/m/${token}$`));

  const href = await panel.getByTestId("invite-whatsapp").getAttribute("href");
  expect(href).toMatch(/^https:\/\/wa\.me\/\?text=/);
  const text = decodeURIComponent(href!.slice("https://wa.me/?text=".length));
  expect(text).toContain(copied);
  expect(text).toContain("davet");
  // Link URL kodlu: ham "/" ya da ":" bağlantıda geçmez.
  expect(href!.slice("https://wa.me/?text=".length)).not.toContain("://");

  // Bekleyen davetler listesinde de aynı düğmeler.
  const row = section.getByTestId("app-invite-row").filter({ hasText: email });
  await expect(row.getByTestId("invite-copy")).toBeVisible();
  await expect(row.getByTestId("invite-whatsapp")).toBeVisible();
});

test("e-posta gönderilemezse uyarı çıkar, davet ve link kalır", async ({ page }) => {
  const section = await openSection(page);
  // Sunucunun e-posta ayarı eksikmiş gibi: gerçek yanıtı alıp yalnız
  // e-posta durumunu değiştir.
  await page.route("**/v1/tenant/engagements/*/invitations", async (route) => {
    if (route.request().method() !== "POST") return route.fallback();
    const response = await route.fetch();
    const json = await response.json();
    json.data.email_sent = false;
    json.data.email_error = "mail_not_configured";
    await route.fulfill({ response, json });
  });
  const email = uniqueEmail("davetlinki-hata");
  await invite(page, email);

  const warning = section.getByTestId("invite-email-warning");
  await expect(warning).toBeVisible();
  await expect(warning).toContainText("E-posta gönderilemedi, bağlantıyı kopyalayıp paylaşın");
  const panel = section.getByTestId("invite-link-panel");
  await panel.getByTestId("invite-copy").click();
  await expect(panel.getByTestId("invite-copy")).toHaveText("Kopyalandı");
  const copied = await page.evaluate(() => navigator.clipboard.readText());
  expect(copied).toContain("/invite/m/");

  // Davet oluştu ve link çalışıyor.
  const list = await api.json<{ items: { email: string; status: string }[] }>(
    await api.get(`/v1/tenant/engagements/${engagement}/invitations`),
  );
  expect(list.items.find((x) => x.email === email)?.status).toBe("pending");
  const token = copied.slice(copied.lastIndexOf("/") + 1);
  const preview = await api.post("/v1/taxpayer-invitations/preview", { token });
  expect(preview.status(), await preview.text()).toBe(200);
});

test("bekleyen davet yeniden gönderilince yeni link çıkar, eskisi çalışmaz", async ({ page }) => {
  const email = uniqueEmail("davetlinki-yeniden");
  const created = await api.post(`/v1/tenant/engagements/${engagement}/invitations`, {
    email,
    role: "employee",
  });
  expect(created.status(), await created.text()).toBe(201);
  const first = (JSON.parse(await created.text()) as { data: { invite_url: string } }).data;
  expect(first.invite_url).toContain("/invite/m/");

  const section = await openSection(page);
  const row = section.getByTestId("app-invite-row").filter({ hasText: email });
  await expect(row).toBeVisible();
  // Link saklanmaz: sayfa yeniden açılınca kopyala düğmesi yok, yeniden gönder var.
  await expect(row.getByTestId("invite-copy")).toHaveCount(0);
  await row.getByTestId("invite-resend").click();
  await expect(row.getByTestId("invite-copy")).toBeVisible();
  await row.getByTestId("invite-copy").click();
  const copied = await page.evaluate(() => navigator.clipboard.readText());
  expect(copied).toContain("/invite/m/");
  expect(copied).not.toBe(first.invite_url);

  const oldToken = first.invite_url.slice(first.invite_url.lastIndexOf("/") + 1);
  const old = await api.post("/v1/taxpayer-invitations/preview", { token: oldToken });
  expect(old.status()).toBe(404);
});

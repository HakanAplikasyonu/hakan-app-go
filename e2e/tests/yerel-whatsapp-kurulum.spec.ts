import { expect, test } from "@playwright/test";

import { DEMO, officePath, uiLogin } from "./lib";

/**
 * WhatsApp uzantısı kolay kurulum (#780): paket frontend derlemesinde
 * üretilir ve sabit adresten iner; WhatsApp sayfasında 4 adımlı kurulum,
 * chrome://extensions kopyalama düğmesi ve uzantının canlı durumu.
 *
 * Veri: demo ofisi, yalnızca okur. "Bağlandı" durumu için oturum listesi
 * sahte yanıtla verilir (gerçek uzantı eşleştirilmez).
 */
const ZIP = "/indir/easytrade-whatsapp.zip";

test("paket sabit adresten iner ve içinde manifest.json var", async ({ request }) => {
  const res = await request.get(ZIP);
  expect(res.status()).toBe(200);
  const body = await res.body();
  expect(body.subarray(0, 2).toString("latin1")).toBe("PK");
  expect(body.includes(Buffer.from("manifest.json"))).toBe(true);

  const info = await request.get("/indir/easytrade-whatsapp.json");
  expect(info.status()).toBe(200);
  const json = (await info.json()) as { version: string; size: number };
  expect(json.version).toMatch(/^\d+\.\d+\.\d+$/);
  expect(json.size).toBe(body.length);
});

test("kurulum adımları, indirme, kopyalama ve bağlantı durumu", async ({ page }) => {
  await page.context().grantPermissions(["clipboard-read", "clipboard-write"]);
  await uiLogin(page, DEMO.owner);

  // Önce eşleşmemiş: uzantı oturumu yok.
  let connected = false;
  await page.route("**/api/v1/auth/sessions", async (route) => {
    const items = [
      {
        uuid: "00000000-0000-4000-8000-000000000001",
        current: true,
        user_agent: "Mozilla/5.0 Chrome",
        ip_address: null,
        created_at: new Date().toISOString(),
        expires_at: new Date(Date.now() + 86_400_000).toISOString(),
        impersonated: false,
      },
    ];
    if (connected) {
      items.push({
        ...items[0]!,
        uuid: "00000000-0000-4000-8000-000000000002",
        current: false,
        user_agent: "HAKAN WhatsApp Web uzantısı · Ofis",
        created_at: new Date(Date.now() - 120_000).toISOString(),
      });
    }
    await route.fulfill({ json: { success: true, data: { items }, meta: {} } });
  });

  await page.goto(officePath("/whatsapp"));
  const steps = page.getByRole("list", { name: "Kurulum adımları" });
  for (const name of ["1. İndir", "2. Chrome'a ekle", "3. Eşleştir", "4. Dene"]) {
    await expect(steps.getByRole("button", { name })).toBeVisible();
  }
  await expect(page.getByTestId("whatsapp-extension-status")).toContainText("Bekleniyor");

  // 1. İndir: büyük düğme, sürüm ve boyut.
  await expect(page.getByTestId("whatsapp-package-info")).toContainText(/Sürüm \d+\.\d+\.\d+ · .*(KB|MB|B)/);
  const download = page.waitForEvent("download");
  await page.getByRole("link", { name: "Uzantıyı indir" }).click();
  expect((await download).suggestedFilename()).toBe("easytrade-whatsapp.zip");

  // 2. Chrome'a ekle: chrome://extensions kopyalanır; adım yenilemede korunur.
  await page.getByRole("button", { name: "Sonraki adım" }).click();
  await expect(page.getByTestId("whatsapp-chrome-url")).toHaveText("chrome://extensions");
  await page.getByRole("button", { name: "Kopyala" }).click();
  expect(await page.evaluate(() => navigator.clipboard.readText())).toBe("chrome://extensions");
  await page.reload();
  await expect(page.getByTestId("whatsapp-chrome-url")).toBeVisible();

  // 3. Eşleştir: kod düğmesi bu adımda.
  await steps.getByRole("button", { name: "3. Eşleştir" }).click();
  await expect(page.getByRole("button", { name: "Eşleştirme kodu al" })).toBeVisible();

  // Uzantı eşleşince durum yeşil "Bağlandı" olur ve "Dene" adımına geçilir.
  connected = true;
  await expect(page.getByTestId("whatsapp-extension-status")).toContainText("Bağlandı ✓", { timeout: 15_000 });
  await expect(page.getByTestId("whatsapp-extension-status")).toContainText("son görülme");
  await expect(page.getByRole("link", { name: "WhatsApp Web'i aç" })).toBeVisible();
});

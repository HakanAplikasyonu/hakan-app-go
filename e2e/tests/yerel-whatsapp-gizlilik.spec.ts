import { expect, test } from "@playwright/test";

/**
 * WhatsApp Web uzantısının gizlilik bildirimi (#780). Chrome Web Store
 * listelemesi bu adrese bağlanır; oturum olmadan açılmalı (girişe
 * yönlendirmemeli). Yalnızca okur: diğer dosyalarla paralel koşar.
 */
const PATH = "/gizlilik/whatsapp-uzantisi";

test("oturumsuz istek 200 döner, girişe yönlendirmez", async ({ request }) => {
  const res = await request.get(PATH, { maxRedirects: 0 });
  expect(res.status()).toBe(200);
  expect(res.headers()["content-type"]).toContain("text/html");
});

test("sayfa oturumsuz açılır ve uzantının gönderdiklerini anlatır", async ({ browser }) => {
  const ctx = await browser.newContext({ storageState: { cookies: [], origins: [] } });
  const page = await ctx.newPage();
  const res = await page.goto(PATH);
  expect(res?.status()).toBe(200);
  await expect(page).toHaveURL(new RegExp(`${PATH}$`));
  await expect(page.getByRole("heading", { level: 1, name: /WhatsApp uzantısı gizlilik bildirimi/ })).toBeVisible();
  await expect(page.getByRole("heading", { name: "Ofisinizin sunucusuna gönderilenler" })).toBeVisible();
  await expect(page.getByText("Mesaj metinleri okunmaz ve gönderilmez.")).toBeVisible();
  await expect(page.getByRole("link", { name: "Aydınlatma Metni" })).toHaveAttribute("href", "/legal/privacy_notice");
  await ctx.close();
});

import { expect, test } from "@playwright/test";

import {
  addConsentedTaxpayer,
  Api,
  DEMO,
  officePath,
  uiLogin,
  uniqueVkn,
} from "./lib";

/**
 * Easytrade bağlantı formu (#780).
 *
 * - Easytrade sekmesinde bağlantı yokken form görünür; firma alanının adı
 *   Easytrade'deki gibi "ID", altında "nereden bulunur" ipucu var.
 * - ID rakam değilse alan hemen uyarır ve butonlar kapanır.
 * - "Bağlantıyı test et" uydurma kullanıcıyla Easytrade'i bir kez çağırır,
 *   hiçbir şey kaydetmez ve nedeni Türkçe gösterir (yanlış e-posta/şifre,
 *   servis yanıt vermiyor ya da zaman aşımı) + kısa teknik kod.
 * - Uç (`POST …/easytrade-connections/test`) rakam olmayan ID'yi 422
 *   `INVALID_COMPANY_ID` ile, eksik gövdeyi 409 ile geri çevirir.
 *
 * Veri: her koşuda yeni mükellef; gerçek Easytrade kullanıcısı yok.
 */
test.describe.configure({ mode: "serial" });

const run = `${Date.now()}`.slice(-6);
const taxpayer = { vkn: uniqueVkn(), title: `Easytrade Bağlantı ${run} A.Ş.` };
const TURKISH =
  /e-postası ya da şifresi yanlış|servis yanıt vermiyor|yanıt vermiyor|zaman aşımı|eşleşmiyor|yetkisi yok/;

test.describe("Easytrade bağlantı formu (#780)", () => {
  let engagement = "";
  let api: Api;
  const base = () =>
    `/v1/tenant/engagements/${engagement}/easytrade-connections`;

  test.beforeAll(async ({ browser }) => {
    test.setTimeout(240_000);
    api = await Api.login(DEMO.owner);
    engagement = await addConsentedTaxpayer(
      browser,
      api,
      taxpayer,
      "easytrade",
    );
  });

  test.afterAll(async () => {
    await api?.dispose();
  });

  test("ID alanı, ipucu ve test butonu Türkçe hata gösterir", async ({
    page,
  }) => {
    test.setTimeout(180_000);
    await uiLogin(page, DEMO.owner);
    await page.goto(officePath(`/engagements/${engagement}/easytrade`));

    const id = page.getByLabel("ID", { exact: true });
    await expect(id).toBeVisible();
    await expect(page.getByText("Şirket kimliği")).toHaveCount(0);
    await expect(page.locator("#et-company_id-hint")).toContainText(
      "Easytrade'de firma bilgilerinde",
    );

    await page
      .getByLabel("Easytrade e-postası")
      .fill(`e2e-${run}@example.test`);
    await page.getByLabel("Şifre", { exact: true }).fill("uydurma-sifre");

    await id.fill("abc");
    await expect(page.locator("#et-company_id-hint")).toHaveText(
      "ID yalnızca rakamlardan oluşur.",
    );
    const testButton = page.getByRole("button", { name: "Bağlantıyı test et" });
    await expect(testButton).toBeDisabled();

    await id.fill("4242");
    await expect(testButton).toBeEnabled();
    await testButton.click();

    const outcome = page.getByTestId("et-outcome");
    await expect(outcome).toBeVisible({ timeout: 90_000 });
    await expect(outcome).toHaveAttribute("role", "alert");
    await expect(outcome).toContainText(TURKISH);
    await expect(outcome).toContainText(/Teknik kod: EASYTRADE_[A-Z_]+/);
    await expect(outcome).not.toContainText(/request failed|refused|answered/i);

    // Hiçbir şey kaydedilmedi.
    const list = await api.json<{ items: unknown[] }>(await api.get(base()));
    expect(list.items).toHaveLength(0);
  });

  test("uç geçersiz ID'yi ve eksik gövdeyi geri çevirir", async () => {
    const bad = await api.post(`${base()}/test`, {
      email: "a@example.test",
      password: "x",
      company_id: "abc",
    });
    expect(bad.status()).toBe(422);
    expect((await bad.json()).error.code).toBe("INVALID_COMPANY_ID");

    const empty = await api.post(`${base()}/test`, {});
    expect(empty.status()).toBe(409);
  });
});

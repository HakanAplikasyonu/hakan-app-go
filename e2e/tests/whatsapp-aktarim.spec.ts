import { expect, test } from "@playwright/test";

import { Api, DEMO, engagementOf, officePath, uiLogin } from "./lib";

/**
 * WhatsApp uzantısı sayfası (#388) ve yevmiyede programa aktarım geçmişi
 * (#391) (#452).
 *
 * Veri: demo ofisi, Atlas Gıda; yalnızca okur. Demo tohumu WhatsApp'tan iki
 * kayıt (gider, belge) ve Ekim 2026 için iki aktarım (Luca aktarıldı, Zirve
 * klasör hatası) bırakır. Eşleştirme kodu almak tek kullanımlık bir kod
 * üretir; başka kayda dokunmaz.
 */
test.describe("WhatsApp uzantısı", () => {
  test("eşleştirme kodu ve kayıt türü filtresi", async ({ page }) => {
    await uiLogin(page, DEMO.owner);
    await page.goto(officePath("/whatsapp"));
    await expect(page.getByRole("heading", { level: 1, name: "WhatsApp uzantısı" })).toBeVisible();

    await page.getByRole("button", { name: "Eşleştirme kodu al" }).click();
    await expect(page.getByText(/10 dakika|geçerli/).first()).toBeVisible();
    await expect(page.locator("code").filter({ hasText: /^[A-Z0-9-]{6,}$/ }).first()).toBeVisible();

    const saved = page.getByRole("region", { name: "WhatsApp'tan kaydedilenler" });
    const rows = saved.getByRole("table").getByRole("row").filter({ hasText: DEMO.atlas.title });
    await expect(rows.filter({ hasText: "Gider" }).first()).toBeVisible();
    await expect(rows.filter({ hasText: "Belge" }).first()).toBeVisible();

    const kind = saved.getByRole("group", { name: "Kayıt türü" });
    await kind.getByRole("button", { name: "Gider" }).click();
    await expect(rows.filter({ hasText: "Belge" })).toHaveCount(0);
    await expect(rows.filter({ hasText: "Gider" }).first()).toBeVisible();
    await kind.getByRole("button", { name: "Belge" }).click();
    await expect(rows.filter({ hasText: "Gider" })).toHaveCount(0);
    await expect(rows.filter({ hasText: "Belge" }).first()).toBeVisible();
    await kind.getByRole("button", { name: "Tümü" }).click();
    await expect(rows.filter({ hasText: "Gider" }).first()).toBeVisible();
  });
});

test.describe("programa aktarım", () => {
  test("yevmiyede aktarım geçmişi ve hata ekranı", async ({ page }) => {
    const api = await Api.login(DEMO.owner);
    const atlas = await engagementOf(api, DEMO.atlas.title);
    await api.dispose();

    await uiLogin(page, DEMO.owner);
    await page.goto(officePath(`/journals?taxpayer=${atlas}`));
    // Dönem ay seçiciyle (takvim düğmesi → ay).
    await page.getByRole("button", { name: "Dönem", exact: true }).click();
    await expect(page.getByText("2026", { exact: true })).toBeVisible();
    await page.getByRole("button", { name: "Eki", exact: true }).click();
    await expect(page.getByRole("heading", { name: "Programa aktarım" })).toBeVisible();

    const history = page.getByRole("row");
    await expect(history.filter({ hasText: /Luca.*Programa aktarıldı/ }).first()).toBeVisible();
    const failed = history.filter({ hasText: /Zirve.*Hata verdi/ }).first();
    await expect(failed).toContainText("Aktarım klasörü bulunamadı");
    await expect(failed.getByRole("button", { name: "Yeniden dene" })).toBeVisible();
    await expect(page.getByRole("alert").filter({ hasText: /aktarım hata verdi/ })).toBeVisible();
  });
});

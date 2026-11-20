import { expect, test } from "@playwright/test";

import { Api, DEMO, engagementOf, officePath, pickTaxpayer, uiLogin, openNewEntry } from "./lib";

/**
 * Onaylanmış ama henüz programa aktarılmamış fişin onayı gerekçeyle geri
 * alınır (#566): fiş taslağa döner, neden geri alındığı listede görünür,
 * düzeltilip yeniden onaylanabilir. Gerekçesiz geri alma sunucuda reddedilir.
 *
 * Veri: demo ofisi, Selin Yılmaz. Dosya yalnızca kendi açtığı, benzersiz
 * açıklamalı fişe dokunur; fişi aktarmaz.
 */
const description = `E2E onay geri alma ${Date.now()}`;
const reason = "Hesap kodu yanlış, düzeltilecek";
// Benzersiz tutar: aynı tutarlı başka bir fişle karışmasın.
const amount = `${100 + (Date.now() % 800)},${String(Date.now() % 100).padStart(2, "0")}`;

test("onaylı fişin onayı gerekçeyle geri alınır, fiş taslağa döner ve yeniden onaylanır (#566)", async ({ page }) => {
  await uiLogin(page, DEMO.owner);
  await page.goto(officePath("/journals"));
  await pickTaxpayer(page, DEMO.selin.title);

  // Dengeli iki satırlı fiş: kaydet ve onayla.
  await openNewEntry(page);
  await page.getByPlaceholder(/açıklama/i).first().fill(description);
  const lines = page.locator('tbody tr:has(input[placeholder="770.01"])');
  await lines.nth(0).locator("input").nth(0).fill("770.01");
  await lines.nth(0).locator("input").nth(2).fill(amount);
  await lines.nth(1).locator("input").nth(0).fill("100.01");
  await page.getByRole("button", { name: "Farkı kapat" }).click();
  await expect(page.getByText("Dengeli", { exact: true })).toBeVisible();
  await page.getByRole("button", { name: "Taslağı kaydet" }).click();

  const row = page.locator("tr", { hasText: description }).first();
  await expect(row).toBeVisible();
  await row.getByRole("button", { name: /^Onayla/ }).click();
  await expect(row).toContainText("Onaylı");

  // Sunucu: gerekçesiz geri alma reddedilir, fiş onaylı kalır.
  const api = await Api.login(DEMO.owner);
  const selin = await engagementOf(api, DEMO.selin.title);
  const base = `/v1/tenant/engagements/${selin}/journal-drafts`;
  const find = async () => {
    const list = await api.json<{
      items: { uuid: string; description: string; status: string; approval_reverted?: { reason: string } }[];
    }>(await api.get(`${base}?limit=200`));
    return list.items.find((d) => d.description === description)!;
  };
  const draft = await find();
  expect(draft.status).toBe("approved");
  const noReason = await api.post(`${base}/${draft.uuid}/unapprove`, { reason: "  " });
  expect(noReason.status()).toBe(400);
  expect(await noReason.text()).toContain("REASON_REQUIRED");
  expect((await find()).status).toBe("approved");

  // Ekran: "Onayı geri al" gerekçe ister; fiş taslağa döner, neden görünür.
  await row.getByRole("button", { name: "Onayı geri al" }).click();
  const dialog = page.getByRole("alertdialog").or(page.getByRole("dialog"));
  await expect(dialog).toContainText("Onayı geri alma nedeni");
  await dialog.getByRole("textbox").fill(reason);
  await dialog.getByRole("button", { name: "Onayı geri al" }).click();
  await expect(row).toContainText("Taslak");
  await expect(row.getByTestId("approval-reverted")).toHaveText(`Onay geri alındı: ${reason}`);

  // Veritabanı: taslak, gerekçe kayıtlı.
  const reverted = await find();
  expect(reverted.status).toBe("draft");
  expect(reverted.approval_reverted?.reason).toBe(reason);

  // Taslak yeniden onaylanabilir; taslağın onayı geri alınamaz (409).
  const again = await api.post(`${base}/${draft.uuid}/unapprove`, { reason });
  expect(again.status()).toBe(409);
  await row.getByRole("button", { name: /^Onayla/ }).click();
  await expect(row).toContainText("Onaylı");
  expect((await find()).status).toBe("approved");
  await api.dispose();
});

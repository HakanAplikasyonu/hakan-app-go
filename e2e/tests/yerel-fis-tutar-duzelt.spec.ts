import { expect, test } from "@playwright/test";

import { addConsentedTaxpayer, Api, DEMO, officePath, uiLogin, uniqueVkn } from "./lib";

/**
 * #566: borç-alacak tutmayan taslak fiş kontrol ekranında düzeltilir.
 * Fişin engeli "Borç ve alacak tutmuyor; fark …" → "Tutarları düzelt" →
 * elle girilmiş satırın alacağı yazılır, fark canlı sıfırlanır → kaydet →
 * fiş kontrolden geçer → onaylanır.
 *
 * Veri: demo ofisine her koşuda yeni bir mükellef ve API ile tek taslak fiş
 * (borç 100, alacak 90); başka dosyanın kaydına dokunmaz.
 */
const run = `${Date.now()}`.slice(-6);
const taxpayer = { vkn: uniqueVkn(), title: `Tutar Ticaret ${run} A.Ş.` };
const description = `E2E tutar düzeltme ${run}`;

/** Beyan dönemi (önceki ay): Fişler ekranı varsayılan olarak bunu gösterir. */
function declarationPeriod(now = new Date()) {
  const d = new Date(Date.UTC(now.getUTCFullYear(), now.getUTCMonth() - 1, 15));
  return { period: d.toISOString().slice(0, 7), date: d.toISOString() };
}

test("Dengesiz taslak: Tutarları düzelt → kontrol tamam → onayla (#566)", async ({ browser, page }) => {
  test.setTimeout(240_000);
  const api = await Api.login(DEMO.owner);
  const engagement = await addConsentedTaxpayer(browser, api, taxpayer, "tutar");
  const base = `/v1/tenant/engagements/${engagement}`;

  const { period, date } = declarationPeriod();
  const created = await api.post(`${base}/journal-drafts`, {
    entry_date: date,
    description,
    lines: [
      { account: "770.01", debit: "100.00", credit: "0" },
      { account: "320.01", debit: "0", credit: "90.00" },
    ],
  });
  expect(created.status(), await created.text()).toBeLessThan(300);
  const list = await api.json<{ items: { uuid: string; description: string; status: string; period: string }[] }>(
    await api.get(`${base}/journal-drafts?limit=50`),
  );
  const draft = list.items.find((d) => d.description === description)!;
  expect(draft).toMatchObject({ status: "draft", period });

  // API: iki taraf birden dolu satır reddedilir.
  const both = await api.patch(`${base}/journal-drafts/${draft.uuid}/lines/2`, {
    debit: "10.00",
    credit: "90.00",
  });
  expect(both.status()).toBe(400);

  await uiLogin(page, DEMO.owner);
  await page.goto(officePath(`/journals?taxpayer=${engagement}`));
  const row = page.locator("tr", { hasText: description }).first();
  await expect(row).toBeVisible();
  await expect(row).toContainText("Borç ve alacak tutmuyor");

  // Pencere: satırlar ve fark görünür; alacak 100 yazılınca fark kapanır.
  await row.getByRole("button", { name: "Tutarları düzelt" }).click();
  const dialog = page.getByRole("dialog");
  await expect(dialog.getByTestId("draft-amounts-diff")).toContainText("Fark");
  await expect(dialog.getByRole("button", { name: "Kaydet" })).toBeDisabled();
  const credit = dialog.getByRole("textbox", { name: "Satır 2 alacak" });
  await credit.fill("100,00");
  await expect(dialog.getByTestId("draft-amounts-diff")).toHaveText("Borç ve alacak tutuyor.");
  // Bir satırda iki taraf dolarsa kaydedilmez.
  await dialog.getByRole("textbox", { name: "Satır 2 borç" }).fill("5");
  await expect(dialog.getByRole("button", { name: "Kaydet" })).toBeDisabled();
  await dialog.getByRole("textbox", { name: "Satır 2 borç" }).fill("");
  await dialog.getByRole("button", { name: "Kaydet" }).click();
  await expect(dialog).toBeHidden();
  await expect(row).toContainText("Kontrol tamam");

  await row.getByRole("button", { name: /^Onayla/ }).click();
  await expect(row).toContainText("Onaylı");

  // Veritabanı: fiş onaylı, satır 2 alacağı 100.
  const detail = await api.json<{ status: string; items: { line_no: number; debit: string; credit: string }[] }>(
    await api.get(`${base}/journal-drafts/${draft.uuid}`),
  );
  expect(detail.status).toBe("approved");
  const line2 = detail.items.find((l) => l.line_no === 2)!;
  expect([Number(line2.debit), Number(line2.credit)]).toEqual([0, 100]);
  await api.dispose();
});

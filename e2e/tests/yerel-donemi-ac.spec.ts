import { expect, test } from "@playwright/test";

import { addConsentedTaxpayer, Api, DEMO, officePath, uiLogin, uniqueVkn } from "./lib";

/**
 * Kapalı dönemdeki taslak fiş (#669 A): "Dönem kapalı; onay için önce dönemi
 * açın" mesajının yanındaki "Dönemi aç" düğmesi dönemi gerekçesiyle açar;
 * fiş kontrolden geçer ve onaylanır. Gerekçe dönemin kilit geçmişinde kalır.
 *
 * Veri: demo ofisine her koşuda yeni bir mükellef; taslak fiş ve dönem kilidi
 * API ile. Başka dosyanın kaydına ve dönemine dokunmaz.
 */
const run = `${Date.now()}`.slice(-6);
const taxpayer = { vkn: uniqueVkn(), title: `Dönem Aç ${run} A.Ş.` };
const description = `E2E kapali donem ${run}`;
const reason = `Unutulan fis eklenecek (${run})`;

/** Beyan dönemi (önceki ay) ve dönem şeridindeki etiketi ("Eki 26"). */
function declarationPeriod(now = new Date()) {
  const d = new Date(Date.UTC(now.getUTCFullYear(), now.getUTCMonth() - 1, 15));
  const label = new Intl.DateTimeFormat("tr-TR", { month: "short", year: "2-digit", timeZone: "UTC" }).format(d);
  return { period: d.toISOString().slice(0, 7), date: d.toISOString(), label };
}

test("kapalı dönemdeki fişte 'Dönemi aç' dönemi gerekçeyle açar; fiş onaylanır (#669)", async ({ browser, page }) => {
  test.setTimeout(240_000);
  const api = await Api.login(DEMO.owner);
  const engagement = await addConsentedTaxpayer(browser, api, taxpayer, "donem");
  const base = `/v1/tenant/engagements/${engagement}`;
  const { period, date, label } = declarationPeriod();

  // Dengeli taslak fiş, sonra dönem kilitlenir: fiş onaylanamaz hâle gelir.
  const created = await api.post(`${base}/journal-drafts`, {
    entry_date: date,
    description,
    lines: [
      { account: "770.01", debit: "150.00", credit: "0" },
      { account: "100.01", debit: "0", credit: "150.00" },
    ],
  });
  expect(created.status(), await created.text()).toBeLessThan(300);
  const locked = await api.post(`${base}/periods/${period}/lock`);
  expect(locked.status(), await locked.text()).toBe(200);
  const find = async () => {
    const list = await api.json<{
      items: { uuid: string; description: string; status: string; issues?: { code: string }[] }[];
    }>(await api.get(`${base}/journal-drafts?period=${period}&limit=50`));
    return list.items.find((d) => d.description === description)!;
  };
  const draft = await find();
  expect(draft.issues?.map((i) => i.code)).toContain("period_locked");
  // Sunucu da engeller: kapalı dönemde onay 422, gerekçesiz açma 400.
  expect((await api.post(`${base}/journal-drafts/${draft.uuid}/approve`)).status()).toBe(422);
  expect((await api.post(`${base}/periods/${period}/unlock`, { reason: "" })).status()).toBe(400);

  // Ekran: fişin satırında engel ve "Dönemi aç".
  await uiLogin(page, DEMO.owner);
  await page.goto(officePath(`/engagements/${engagement}/journals`));
  await page.getByRole("button", { name: new RegExp(`^${label}`) }).click();
  const row = page.locator("tr", { hasText: description }).first();
  await expect(row).toBeVisible();
  await expect(row).toContainText("Dönem kapalı; onay için önce dönemi açın");
  await expect(row.getByRole("button", { name: /^Onayla/ })).toBeDisabled();
  await row.getByRole("button", { name: "Dönemi aç" }).click();
  const dialog = page.getByRole("alertdialog").or(page.getByRole("dialog"));
  await expect(dialog).toContainText("Açma nedenini yazın");
  await dialog.getByRole("textbox").fill(reason);
  await dialog.getByRole("button", { name: "Dönemi aç" }).click();

  // Dönem açıldı: engel kalkar, fiş onaylanır.
  await expect(row).toContainText("Kontrol tamam");
  await expect(row).not.toContainText("Dönem kapalı");
  await row.getByRole("button", { name: /^Onayla/ }).click();
  await expect(row).toContainText("Onaylı");

  // Veritabanı: açma gerekçesi kilit geçmişinde, fiş onaylı.
  const events = await api.json<{ items: { action: string; reason?: string }[] }>(
    await api.get(`${base}/periods/${period}/lock-events`),
  );
  expect(events.items.find((e) => e.action === "unlock")?.reason).toBe(reason);
  expect((await find()).status).toBe("approved");
  await api.dispose();
});

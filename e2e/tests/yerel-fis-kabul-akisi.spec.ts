import { expect, test } from "@playwright/test";

import { addConsentedTaxpayer, Api, DEMO, officePath, uiLogin, uniqueVkn } from "./lib";

/**
 * #566 kabul senaryosu uçtan uca: Sıradaki İşler → "Taslak fişleri kontrol
 * et" → kontrol ekranı → engelli fiş (kapalı dönem) "Dönemi aç" ile
 * düzeltilir → onaylanır → Sıradaki İşler'e geri dönülür → "Aktarımı kontrol
 * et" → aktarım bölümü → fiş seçilip Luca'ya aktarılır. Hiçbir Sıradaki İş
 * düğmesi doğrudan onaylamaz ya da aktarmaz; her adımın sonucu API'den de
 * doğrulanır.
 *
 * Veri: demo ofisine her koşuda yeni bir mükellef; tek dengeli taslak fiş
 * ve dönem kilidi API ile. Başka dosyanın kaydına dokunmaz.
 */
const run = `${Date.now()}`.slice(-6);
const taxpayer = { vkn: uniqueVkn(), title: `Kabul Akışı ${run} A.Ş.` };
const description = `E2E kabul akışı ${run}`;

/** Beyan dönemi (önceki ay): Sıradaki İş bu dönemi sayar. */
function declarationPeriod(now = new Date()) {
  const d = new Date(Date.UTC(now.getUTCFullYear(), now.getUTCMonth() - 1, 15));
  return { period: d.toISOString().slice(0, 7), date: d.toISOString() };
}

type Draft = { uuid: string; description: string; status: string; issues?: { code: string }[] };

test("Sıradaki İşler → kontrol → dönemi aç → onayla → aktarım → aktar (#566)", async ({ browser, page }) => {
  test.setTimeout(300_000);
  const api = await Api.login(DEMO.owner);
  const engagement = await addConsentedTaxpayer(browser, api, taxpayer, "kabul");
  const base = `/v1/tenant/engagements/${engagement}`;
  const { period, date } = declarationPeriod();

  const created = await api.post(`${base}/journal-drafts`, {
    entry_date: date,
    description,
    lines: [
      { account: "770.01", debit: "175.00", credit: "0" },
      { account: "100.01", debit: "0", credit: "175.00" },
    ],
  });
  expect(created.status(), await created.text()).toBeLessThan(300);
  expect((await api.post(`${base}/periods/${period}/lock`)).status()).toBe(200);
  const find = async () =>
    (await api.json<{ items: Draft[] }>(await api.get(`${base}/journal-drafts?period=${period}&limit=50`))).items.find(
      (d) => d.description === description,
    )!;
  expect((await find()).issues?.map((i) => i.code)).toContain("period_locked");

  // 1) Sıradaki İşler: düğme kontrol ekranına götürür, onaylamaz.
  await uiLogin(page, DEMO.owner);
  const nextWork = async (button: string) => {
    await page.goto(officePath("/copilot"));
    await expect(page.getByRole("heading", { name: "Sıradaki iş" })).toBeVisible();
    await page.getByPlaceholder("Mükellef ya da VKN/TCKN ara").fill(taxpayer.title);
    const work = page.getByRole("row").filter({ hasText: taxpayer.title });
    await expect(work).toHaveCount(1);
    await work.getByRole("button", { name: button }).click();
    await page.waitForURL((u) => u.pathname.endsWith("/journals") && u.searchParams.get("taxpayer") === engagement);
  };
  await nextWork("Taslak fişleri kontrol et");
  expect((await find()).status).toBe("draft");

  // 2) Kontrol ekranı: engelli fiş onaylanamaz; "Dönemi aç" ile düzeltilir.
  const row = page.locator("tr", { hasText: description }).first();
  await expect(row).toContainText("Dönem kapalı; onay için önce dönemi açın");
  await expect(row.getByRole("button", { name: /^Onayla/ })).toBeDisabled();
  await row.getByRole("button", { name: "Dönemi aç" }).click();
  const dialog = page.getByRole("alertdialog").or(page.getByRole("dialog"));
  await dialog.getByRole("textbox").fill("Kabul senaryosu: eksik fiş işlenecek");
  await dialog.getByRole("button", { name: "Dönemi aç" }).click();
  await expect(row).toContainText("Kontrol tamam");

  // 3) Onay.
  await row.getByRole("button", { name: /^Onayla/ }).click();
  await expect(row).toContainText("Onaylı");
  const draft = await find();
  expect(draft.status).toBe("approved");

  // 4) Sıradaki İşler'e dön: sıradaki iş aktarımın kontrolü; aktarmaz.
  await nextWork("Aktarımı kontrol et");
  expect(new URL(page.url()).hash).toBe("#aktarim");
  await expect(page.getByRole("heading", { name: "Programa aktarım" })).toBeVisible();
  const approved = page.locator("tr", { hasText: description }).first();
  await expect(approved).toContainText("Onaylı");
  await expect(approved).not.toContainText("Aktarıldı");

  // 5) Aktarım: yalnız bu fiş seçilir ve Luca'ya aktarılır.
  await approved.getByRole("checkbox").check();
  await expect(page.getByText("Yalnız tabloda seçilen 1 onaylı fiş aktarılır.")).toBeVisible();
  await page.locator('select:has(option[value="luca"])').selectOption("luca");
  await page.getByRole("button", { name: /^(Seçili 1 fişin çıktısını al|Çıktı al)$/ }).last().click();
  await expect(page.getByText("1 fiş aktarıldı.")).toBeVisible();
  await expect(approved).toContainText("Aktarıldı");

  // Veritabanı: fiş aktarıldı ve bir çıktıya bağlı.
  const detail = await api.json<{ status: string; export?: { exported_at: string } }>(
    await api.get(`${base}/journal-drafts/${draft.uuid}`),
  );
  expect(detail.status).toBe("exported");
  expect(detail.export?.exported_at).toBeTruthy();
  await api.dispose();
});

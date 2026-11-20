import { readFileSync } from "node:fs";
import path from "node:path";

import { expect, test } from "@playwright/test";

import { addConsentedTaxpayer, Api, DEMO, officePath, uiLogin, uniqueVkn } from "./lib";

/**
 * #566 kabul akışı: Sıradaki İşler → "Taslak fişleri kontrol et" → kontrol
 * ekranı → sorunlu fişte "Hesabı düzelt" → fiş kontrolden geçer → onayla.
 * Sıradaki İş düğmesi doğrudan onaylamaz; kullanıcıyı fişlerin kontrol
 * ekranına götürür.
 *
 * Veri: demo ofisine her koşuda yeni bir mükellef. Hesap planı (Luca mizanı)
 * ve hesap planında olmayan bir koda (999.99) yazılmış tek taslak fiş API ile
 * hazırlanır; başka dosyanın kaydına dokunmaz.
 */
const run = `${Date.now()}`.slice(-6);
const taxpayer = { vkn: uniqueVkn(), title: `Kontrol Ticaret ${run} A.Ş.` };
const description = `E2E kontrol akışı ${run}`;
const MIZAN = path.resolve(__dirname, "../../backend/testdata/mizan/oguz-luca-mizan.xlsx");

/** Beyan dönemi (önceki ay): Sıradaki İş ve Fişler ekranı varsayılan olarak bunu gösterir. */
function declarationPeriod(now = new Date()) {
  const d = new Date(Date.UTC(now.getUTCFullYear(), now.getUTCMonth() - 1, 15));
  return { period: d.toISOString().slice(0, 7), date: d.toISOString() };
}

test("Sıradaki İşler → kontrol ekranı → Hesabı düzelt → onayla (#566)", async ({ browser, page }) => {
  test.setTimeout(240_000);
  const api = await Api.login(DEMO.owner);
  const engagement = await addConsentedTaxpayer(browser, api, taxpayer, "kontrol");
  const base = `/v1/tenant/engagements/${engagement}`;

  // Hesap planı: kontrol ancak planı yüklü mükellefte hesap kodunu denetler.
  const imported = await api.upload(`${base}/account-plan/imports`, {
    name: "oguz-luca-mizan.xlsx",
    mimeType: "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet",
    buffer: readFileSync(MIZAN),
  });
  expect(imported.status(), await imported.text()).toBeLessThan(300);
  const plan = await api.json<{ items: { code: string; is_leaf: boolean }[] }>(
    await api.get(`${base}/account-plan/accounts?q=&limit=50`),
  );
  const leaves = plan.items.filter((a) => a.is_leaf).map((a) => a.code);
  expect(leaves.length, "planda alt hesap").toBeGreaterThan(1);
  const [credit, fixed] = leaves;

  // Beyan döneminde, planda olmayan koda yazılmış dengeli taslak fiş.
  const { period, date } = declarationPeriod();
  const created = await api.post(`${base}/journal-drafts`, {
    entry_date: date,
    description,
    lines: [
      { account: "999.99", debit: "150.00", credit: "0" },
      { account: credit, debit: "0", credit: "150.00" },
    ],
  });
  expect(created.status(), await created.text()).toBeLessThan(300);
  const find = async () => {
    const list = await api.json<{ items: { uuid: string; description: string; status: string; period: string }[] }>(
      await api.get(`${base}/journal-drafts?limit=50`),
    );
    return list.items.find((d) => d.description === description)!;
  };
  const draft = await find();
  expect(draft).toMatchObject({ status: "draft", period });

  // Sıradaki İşler: bu mükellefin sıradaki işi fişlerin kontrolü.
  await uiLogin(page, DEMO.owner);
  await page.goto(officePath("/copilot"));
  await expect(page.getByRole("heading", { name: "Sıradaki iş" })).toBeVisible();
  await page.getByPlaceholder("Mükellef ya da VKN/TCKN ara").fill(taxpayer.title);
  const work = page.getByRole("row").filter({ hasText: taxpayer.title });
  await expect(work).toHaveCount(1);
  await work.getByRole("button", { name: "Taslak fişleri kontrol et" }).click();

  // Kontrol ekranı: doğrudan onay yok; fiş listesi ve fişin sorunu görünür.
  await page.waitForURL((u) => u.pathname.endsWith("/journals") && u.searchParams.get("taxpayer") === engagement);
  expect((await find()).status).toBe("draft");
  const row = page.locator("tr", { hasText: description }).first();
  await expect(row).toBeVisible();
  await expect(row).toContainText("Satır 1: 999.99 hesap planında yok");

  // Hesabı düzelt: satırın kodu plandaki bir alt hesaba alınır.
  await row.getByRole("button", { name: "Hesabı düzelt" }).click();
  const dialog = page.getByRole("alertdialog").or(page.getByRole("dialog"));
  await expect(dialog).toContainText("Satır 1 hesap kodu");
  await dialog.getByRole("textbox").fill(fixed);
  await dialog.getByRole("button", { name: "Kaydet" }).click();
  await expect(row).toContainText("Kontrol tamam");
  await expect(row).not.toContainText("999.99");

  // Kontrolden geçen fiş onaylanır.
  await row.getByRole("button", { name: /^Onayla/ }).click();
  await expect(row).toContainText("Onaylı");

  // Veritabanı: fiş onaylı, satır düzeltilen hesapta.
  const detail = await api.json<{ status: string; items: { line_no: number; account: string }[] }>(
    await api.get(`${base}/journal-drafts/${draft.uuid}`),
  );
  expect(detail.status).toBe("approved");
  expect(detail.items.find((l) => l.line_no === 1)?.account).toBe(fixed);
  await api.dispose();
});

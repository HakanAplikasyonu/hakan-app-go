import { expect, test } from "@playwright/test";

import { addConsentedTaxpayer, Api, DEMO, deriveUbl, officePath, uiLogin, uniqueVkn, waitForUploads } from "./lib";

/**
 * Kuyrukta karşı tarafın geçmiş kodları sunucudan (#669 L): bir satırın
 * kodu onaylanınca aynı karşı tarafın öbür satırında "Bu karşı tarafla
 * geçmiş" o kodu gösterir; uç bütün kararları sayar.
 *
 * Veri: demo ofisine her koşuda yeni mükellef; Atlas'ın iki kalemli satış
 * faturası (atlas-01) bu mükellefe türetilir. Başka dosyaya dokunmaz.
 */
const run = `${Date.now()}`.slice(-6);
const taxpayer = { vkn: uniqueVkn(), title: `Geçmiş ${run} A.Ş.` };

type Suggestion = {
  uuid: string;
  status: string;
  target_type: string;
  target_field: string;
  proposed_value: { account_code?: string };
  provenance: { inputs: Record<string, unknown> };
};

test("karşı taraf geçmişi sunucudan gelir (#669 L)", async ({ browser, page }) => {
  test.setTimeout(240_000);
  const api = await Api.login(DEMO.owner);
  const engagement = await addConsentedTaxpayer(browser, api, taxpayer, "gecmis");
  const base = `/v1/tenant/engagements/${engagement}`;

  const up = await api.upload(`${base}/ubl-uploads`, deriveUbl("atlas-01-temel-satis.xml", taxpayer.vkn, taxpayer.title));
  expect(up.status(), await up.text()).toBeLessThan(300);
  await waitForUploads(api, engagement, 1);
  const invoice = (await api.json<{ items: { uuid: string }[] }>(await api.get(`${base}/invoices?limit=5`))).items[0].uuid;
  await api.json(await api.post(`${base}/invoices/${invoice}/account-suggestions`, {}));

  const pending = (
    await api.json<{ items: Suggestion[] }>(await api.get(`${base}/suggestions?status=pending&kind=account_code&limit=100`))
  ).items;
  const lines = pending.filter((s) => s.target_type === "invoice_item" && s.target_field === "account_code");
  // KDV hesabı önerileri kalemle aynı başlığı taşır: kuyrukta karışmasın diye
  // kapatılır (geçmiş yalnız satır hesabını sayar).
  for (const v of pending.filter((s) => s.target_field === "vat_account_code")) {
    await api.json(await api.post(`${base}/suggestions/${v.uuid}/accept`, {}));
  }
  expect(lines.length).toBe(2);
  const key = String(lines[0].provenance.inputs.counterparty_key ?? "");
  expect(key).not.toBe("");
  const code = lines[0].proposed_value.account_code ?? "";
  expect(code).not.toBe("");

  // Öncesi: geçmiş yok. Bir satır onaylanınca kod bir kez sayılır.
  const history = async () =>
    (
      await api.json<{ items: { account_code: string; uses: number }[] }>(
        await api.get(`${base}/suggestions/history?counterparty=${encodeURIComponent(key)}`),
      )
    ).items;
  expect(await history()).toEqual([]);
  await api.json(await api.post(`${base}/suggestions/${lines[0].uuid}/accept`, {}));
  expect(await history()).toEqual([expect.objectContaining({ account_code: code, uses: 1 })]);
  expect((await api.get(`${base}/suggestions/history`)).status()).toBe(400);

  // Ekran: kuyrukta kalan satır seçilince geçmişte o kod görünür.
  await uiLogin(page, DEMO.owner);
  await page.goto(officePath(`/engagements/${engagement}/review`));
  const name = String(lines[1].provenance.inputs.item_name ?? "");
  await page.getByRole("list", { name: "Bekleyen öneriler" }).getByRole("listitem").filter({ hasText: name }).first().click();
  const section = page.locator("section").filter({ has: page.getByRole("heading", { name: "Bu karşı tarafla geçmiş" }) });
  await expect(section).toContainText(code);
  await expect(section).toContainText("1×");
  await api.dispose();
});

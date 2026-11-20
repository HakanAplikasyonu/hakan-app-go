import { expect, test } from "@playwright/test";

import { Api, DEMO, engagementOf, officePath, pickTaxpayer, uiLogin } from "./lib";

/**
 * Fiş onayı → aktarım → mizan (#566, #574): personel taslak hazırlar ama
 * onaylayamaz; sorumlu müşavir onaylar ve seçerek programa aktarır; programa
 * aktarılmış fişin yanında mizan karşılaştırmasının durumu görünür.
 *
 * Veri: demo ofisi. Stajyer (Personel rolü) yalnızca Selin Yılmaz'a atanmış;
 * müşavir (Yönetici) iki mükellefe. Dosya yalnızca kendi açtığı, benzersiz
 * açıklamalı fişe dokunur.
 */
test.describe.configure({ mode: "serial" });

const MUSAVIR = "musavir@demo.hakan.local";
const description = `E2E onay akışı ${Date.now()}`;
// Benzersiz tutar: aynı tutarlı başka bir fiş mükerrer kayıt incelemesini
// açıp aktarımı bekletmesin.
const amount = `${100 + (Date.now() % 800)},${String(Date.now() % 100).padStart(2, "0")}`;

test("personel fişi hazırlar, onaylayamaz (#566)", async ({ page }) => {
  await uiLogin(page, DEMO.intern);
  await page.goto(officePath("/journals"));
  await pickTaxpayer(page, DEMO.selin.title);

  await page.getByPlaceholder(/açıklama/i).first().fill(description);
  const rows = page.locator('tbody tr:has(input[placeholder="770.01"])');
  await rows.nth(0).locator("input").nth(0).fill("770.01");
  await rows.nth(0).locator("input").nth(2).fill(amount);
  await rows.nth(1).locator("input").nth(0).fill("100.01");
  await page.getByRole("button", { name: "Farkı kapat" }).click();
  await expect(page.getByText("Dengeli", { exact: true })).toBeVisible();
  await page.getByRole("button", { name: "Taslağı kaydet" }).click();

  const row = page.locator("tr", { hasText: description }).first();
  await expect(row).toBeVisible();
  await expect(row).toContainText("Müşavir onayı bekliyor");
  await expect(row.getByRole("button", { name: /Onayla/ })).toHaveCount(0);

  // Sunucu da engeller: onay ucu tenant.journals.approve ister.
  const api = await Api.login(DEMO.intern);
  const selin = await engagementOf(api, DEMO.selin.title);
  const list = await api.json<{ items: { uuid: string; description: string }[] }>(
    await api.get(`/v1/tenant/engagements/${selin}/journal-drafts?status=draft&limit=200`),
  );
  const draft = list.items.find((d) => d.description === description);
  expect(draft, "stajyerin taslağı").toBeTruthy();
  const res = await api.post(`/v1/tenant/engagements/${selin}/journal-drafts/${draft!.uuid}/approve`);
  expect(res.status()).toBe(403);
  expect(await res.text()).toContain("tenant.journals.approve");
  await api.dispose();
});

test("müşavir onaylar ve seçerek aktarır (#566, #574)", async ({ page }) => {
  await uiLogin(page, MUSAVIR);
  await page.goto(officePath("/journals"));
  await pickTaxpayer(page, DEMO.selin.title);

  const row = page.locator("tr", { hasText: description }).first();
  await expect(row).toBeVisible();
  await row.getByRole("button", { name: /Onayla/ }).click();
  await expect(row).toContainText("Onaylı");

  // Yalnız bu fiş seçilir; dönemin öbür onaylı fişleri aktarılmaz.
  await row.getByRole("checkbox").check();
  await expect(page.getByText("Yalnız tabloda seçilen 1 onaylı fiş aktarılır.")).toBeVisible();
  await page.locator('select:has(option[value="luca"])').selectOption("luca");
  // Seçim varken düğme "Seçili 1 fişin çıktısını al" yazar.
  await page.getByRole("button", { name: /^(Seçili 1 fişin çıktısını al|Çıktı al)$/ }).last().click();
  await expect(page.getByText("1 fiş aktarıldı.")).toBeVisible();
  await expect(row).toContainText("Aktarıldı");
});

test("programa aktarılan fişte mizan karşılaştırması görünür (#574)", async ({ page }) => {
  const api = await Api.login(DEMO.owner);
  const atlas = await engagementOf(api, DEMO.atlas.title);
  await api.dispose();

  await uiLogin(page, MUSAVIR);
  await page.goto(officePath(`/journals?taxpayer=${atlas}`));
  await page.locator("#journals-period").click();
  await expect(page.getByText("2026", { exact: true })).toBeVisible();
  await page.getByRole("button", { name: "Eki", exact: true }).click();
  await expect(page.getByRole("heading", { name: "Programa aktarım" })).toBeVisible();

  // Demo: Ekim'in Luca aktarımı programa alınmış; yanında mizan durumu.
  const imported = page.getByRole("row").filter({ hasText: /Luca.*Programa aktarıldı/ }).first();
  await expect(imported).toContainText(
    /Luca mizanı kayıtlarla tutuyor|Mizan karşılaştırmasında \d+ hesapta fark var|Mizan aktarımdan önce karşılaştırılmış|Mizanla karşılaştırılmadı/,
  );
  const link = imported.getByRole("button", { name: /Farkları gör|Mizanla karşılaştır/ });
  if (await link.count()) {
    await link.click();
    await page.waitForURL(/\/reconciliation/);
    await expect(page.getByRole("heading", { level: 1, name: DEMO.atlas.title })).toBeVisible();
  }
});

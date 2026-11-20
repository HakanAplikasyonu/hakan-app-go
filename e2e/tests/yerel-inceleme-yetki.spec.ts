import { expect, test } from "@playwright/test";

import { Api, DEMO, officePath, uiLogin } from "./lib";

/**
 * İnceleme kuyruğu karar yetkisi (#569): "Kod yetkiye bağlıysa yetkisi
 * olmayan kullanıcıya nedeni gösterilir."
 *
 * Stajyer (tenant.review.read var, tenant.review.decide yok) kuyruğu ve
 * tabloyu görür; onayla / kodu düzelt / reddet düğmeleri ve toplu onay
 * yerine nedeni yazar, kısayollar karar vermez, tablo salt okunurdur. Sunucu
 * da kararı 403 ile reddeder; öneri bekler durumda kalır. Sahip aynı ekranda
 * düğmeleri görür.
 *
 * Veri: demo ofisinin bekleyen hesap kodu önerileri; dosya karar vermez,
 * hiçbir kaydı değiştirmez.
 */
const NOTE = "Bu kuyruğu yalnızca görebilirsiniz.";

type Suggestion = { uuid: string; engagement_uuid: string; status: string };

test("stajyer kuyruğu görür, karar veremez; nedeni yazılır", async ({ page }) => {
  const intern = await Api.login(DEMO.intern);
  const pending = async () =>
    intern.json<{ items: Suggestion[]; total: number }>(
      await intern.get("/v1/tenant/review/suggestions?status=pending&kind=account_code&limit=1"),
    );
  const before = await pending();
  expect(before.total, "demo ofisinde bekleyen hesap kodu önerisi").toBeGreaterThan(0);
  const s = before.items[0];

  // Sunucu: karar yetkisi yok.
  const denied = await intern.post(`/v1/tenant/engagements/${s.engagement_uuid}/suggestions/${s.uuid}/accept`, {});
  expect(denied.status()).toBe(403);

  // Kuyruk: neden yazılır, karar düğmeleri ve toplu onay yok.
  await uiLogin(page, DEMO.intern);
  await page.goto(officePath("/review"));
  const note = page.getByTestId("review-no-decide");
  await expect(note).toBeVisible();
  await expect(note).toContainText(NOTE);
  await expect(note).toContainText("İnceleme kararı");
  await expect(page.getByRole("button", { name: /^Onayla/ })).toHaveCount(0);
  await expect(page.getByRole("button", { name: /^Kodu düzelt/ })).toHaveCount(0);
  await expect(page.getByRole("button", { name: /^Reddet/ })).toHaveCount(0);
  await expect(page.getByRole("button", { name: /öneriyi toplu onayla/ })).toHaveCount(0);
  // Kısayol karar vermez.
  await page.locator("body").press("a");
  await expect(note).toBeVisible();

  // Tablo: salt okunur, neden yazılır.
  await page.goto(officePath("/review?view=grid"));
  await expect(page.getByTestId("review-no-decide")).toContainText(NOTE);
  const grid = page.getByRole("grid", { name: "Hesap kodu önerileri" });
  const code = grid.locator('[role=row][aria-rowindex="2"] [role=gridcell][aria-colindex="10"]');
  await expect(code).toBeVisible();
  await expect(code).toHaveAttribute("aria-readonly", "true");

  // Veritabanı: öneri hâlâ bekliyor, bekleyen sayısı değişmedi.
  const after = await pending();
  expect(after.total).toBe(before.total);
  const same = await intern.json<{ status: string }>(
    await intern.get(`/v1/tenant/engagements/${s.engagement_uuid}/suggestions/${s.uuid}`),
  );
  expect(same.status).toBe("pending");
  await intern.dispose();
});

test("sahip aynı ekranda karar düğmelerini görür", async ({ page }) => {
  await uiLogin(page, DEMO.owner);
  await page.goto(officePath("/review"));
  await expect(page.getByRole("button", { name: /^Onayla/ }).first()).toBeVisible();
  await expect(page.getByTestId("review-no-decide")).toHaveCount(0);
  await page.goto(officePath("/review?view=grid"));
  await expect(page.getByRole("grid", { name: "Hesap kodu önerileri" })).toBeVisible();
  await expect(page.getByTestId("review-no-decide")).toHaveCount(0);
});

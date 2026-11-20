import { expect, test } from "@playwright/test";

import { Api, DEMO, mailToken, officePath, registerWithToken, uiLogin } from "./lib";

/**
 * İnceleme kuyruğu karar yetkisi (#569): "Kod yetkiye bağlıysa yetkisi
 * olmayan kullanıcıya nedeni gösterilir."
 *
 * Stajyer rolü (tenant.review.read var, tenant.review.decide yok) kuyruğu ve
 * tabloyu görür; onayla / kodu düzelt / reddet düğmeleri ve toplu onay
 * yerine nedeni yazar, kısayollar karar vermez, tablo salt okunurdur. Sunucu
 * da kararı 403 ile reddeder; öneri bekler durumda kalır. Sahip aynı ekranda
 * düğmeleri görür.
 *
 * Veri: demo ofisinin bekleyen hesap kodu önerileri. Demo'daki "Stajyer"
 * (stajyer@) unvanlı bir personeldir ve karar verebilir; bu yüzden her koşuda
 * stajyer rolüyle yeni bir kullanıcı davet edilir ve önerinin mükellefine
 * atanır. Dosya karar vermez, öneriyi değiştirmez.
 */
const run = `${Date.now()}`.slice(-7);
const internEmail = `stajyer-${run}@example.com`;
const password = "Stajyer-e2e-123!";
const NOTE = "Bu kuyruğu yalnızca görebilirsiniz.";

type Suggestion = { uuid: string; engagement_uuid: string; status: string };

test("stajyer kuyruğu görür, karar veremez; nedeni yazılır", async ({ page }) => {
  test.setTimeout(180_000);
  const owner = await Api.login(DEMO.owner);
  const pending = async (api: Api) =>
    api.json<{ items: Suggestion[]; total: number }>(
      await api.get("/v1/tenant/review/suggestions?status=pending&kind=account_code&limit=1"),
    );
  const first = await pending(owner);
  expect(first.total, "demo ofisinde bekleyen hesap kodu önerisi").toBeGreaterThan(0);
  const s = first.items[0];

  // Stajyer rolüyle davet, kayıt ve önerinin mükellefine atama.
  const invited = await owner.post("/v1/tenant/invitations", { email: internEmail, role_key: "intern" });
  expect(invited.status(), await invited.text()).toBeLessThan(300);
  const token = await mailToken(internEmail, "/invite/");
  await registerWithToken(page, token, `/invite/${token}`, { name: "Deniz", surname: "Stajyer", password });
  await page.getByRole("button", { name: "Daveti kabul et" }).click();
  await page.waitForURL((u) => u.pathname.startsWith(officePath()));
  const members = await owner.json<{ items: { user: { uuid: string; email: string } }[] }>(
    await owner.get(`/v1/tenant/members?q=${encodeURIComponent(internEmail)}`),
  );
  const member = members.items.find((m) => m.user.email === internEmail);
  expect(member, "davet edilen stajyer üye").toBeTruthy();
  const assigned = await owner.put(`/v1/tenant/engagements/${s.engagement_uuid}/assignments/${member!.user.uuid}`);
  expect(assigned.status(), await assigned.text()).toBeLessThan(300);

  const intern = await Api.login(internEmail, password);
  const before = await pending(intern);
  expect(before.total).toBeGreaterThan(0);

  // Sunucu: karar yetkisi yok.
  const denied = await intern.post(`/v1/tenant/engagements/${s.engagement_uuid}/suggestions/${s.uuid}/accept`, {});
  expect(denied.status()).toBe(403);

  // Kuyruk: neden yazılır, karar düğmeleri ve toplu onay yok.
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
  const after = await pending(intern);
  expect(after.total).toBe(before.total);
  const same = await intern.json<{ status: string }>(
    await intern.get(`/v1/tenant/engagements/${s.engagement_uuid}/suggestions/${s.uuid}`),
  );
  expect(same.status).toBe("pending");
  await intern.dispose();
  await owner.dispose();
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

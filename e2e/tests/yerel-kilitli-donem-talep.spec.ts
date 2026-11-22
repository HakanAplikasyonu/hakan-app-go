import { expect, test } from "@playwright/test";

import {
  addConsentedTaxpayer,
  Api,
  DEMO,
  deriveUbl,
  officePath,
  uiLogin,
  uniqueVkn,
} from "./lib";

/**
 * Kilitli döneme gelen değişiklik talebinde seçenekler (Oğuz'un demo geri
 * bildirimi): kilitli Ekim 2026'ya üç fatura gelir, üç talep açılır.
 * Ekranda biri reddedilir, biri geçmiş döneme düzeltme olarak işlenir
 * (düzeltme beyannamesi uyarısı görünür), biri ay seçiciyle Aralık 2026'ya
 * uygulanır. Kayıt seçilen kararı ve hedef ayı tutar; Geçmiş dönem takibi
 * sekmesindeki kararlar tablosu bunları cümleyle gösterir. Eski "Uygula"
 * düğmesi yerinde kalır.
 *
 * Veri: demo ofisine her koşuda yeni bir mükellef; dönem kilidi ve
 * faturalar API ile. Ofis geneli ayar değişmez.
 */
const run = `${Date.now()}`.slice(-6);
const taxpayer = { vkn: uniqueVkn(), title: `Kilitli Talep ${run} A.Ş.` };
const LOCKED = "2026-10";
const TARGET = "2026-12";

type ChangeRequest = {
  uuid: string;
  status: string;
  decision: string | null;
  target_period: string | null;
};

test("kilitli dönem talebi: reddet, düzeltme olarak işle ve seçilen aya uygula kaydedilir", async ({
  browser,
  page,
}) => {
  test.setTimeout(300_000);
  const api = await Api.login(DEMO.owner);
  const engagement = await addConsentedTaxpayer(
    browser,
    api,
    taxpayer,
    "kilitlitalep",
  );
  const base = `/v1/tenant/engagements/${engagement}`;
  const locked = await api.post(`${base}/periods/${LOCKED}/lock`);
  expect(locked.status(), await locked.text()).toBe(200);
  for (const file of [
    "atlas-01-temel-satis.xml",
    "atlas-02-ticari-satis.xml",
    "atlas-03-tevkifat.xml",
  ]) {
    const up = await api.upload(
      `${base}/ubl-uploads`,
      deriveUbl(file, taxpayer.vkn, taxpayer.title),
    );
    expect(up.status(), await up.text()).toBeLessThan(300);
  }
  const openList = async () =>
    (
      await api.json<{ items: ChangeRequest[] }>(
        await api.get(`${base}/change-requests?status=open`),
      )
    ).items;
  await expect
    .poll(async () => (await openList()).length, { timeout: 90_000 })
    .toBe(3);

  // Uç: bilinmeyen seçenek ve talebin kendi ayı reddedilir, talep açık kalır.
  const first = (await openList())[0];
  const bad = await api.post(`${base}/change-requests/${first.uuid}/apply`, {
    mode: "later",
  });
  expect(bad.status(), await bad.text()).toBe(400);
  const same = await api.post(`${base}/change-requests/${first.uuid}/apply`, {
    mode: "target_period",
    target_period: LOCKED,
  });
  expect(same.status(), await same.text()).toBe(400);
  expect((await openList()).length).toBe(3);

  await uiLogin(page, DEMO.owner);
  await page.goto(officePath(`/engagements/${engagement}?talepler=ac`));
  const list = page.locator("#change-requests-list");
  await expect(list).toBeVisible();
  const cards = list.getByTestId("change-request");
  await expect(cards).toHaveCount(3);

  // Dört seçenek de görünür; eski "Uygula" yerinde.
  const card = cards.first();
  await expect(card.getByRole("button", { name: "Reddet", exact: true })).toBeVisible();
  await expect(card.getByRole("button", { name: "Uygula", exact: true })).toBeVisible();
  await expect(
    card.getByRole("button", { name: "Geçmiş döneme düzeltme olarak işle", exact: true }),
  ).toBeVisible();
  await expect(
    card.getByRole("button", { name: "Seçilen aya uygula", exact: true }),
  ).toBeVisible();

  // 1) Reddet.
  await card.getByRole("button", { name: "Reddet", exact: true }).click();
  await expect(page.getByText("Talep reddedildi; kayıt değişmedi.")).toBeVisible();
  await expect(cards).toHaveCount(2);

  // 2) Geçmiş döneme düzeltme olarak işle: uyarı, sonra onay.
  await cards
    .first()
    .getByRole("button", { name: "Geçmiş döneme düzeltme olarak işle", exact: true })
    .click();
  const warn = cards.first().getByTestId("change-request-correction");
  await expect(warn).toBeVisible();
  await expect(warn).toContainText("düzeltme beyannamesi");
  await warn
    .getByRole("button", { name: "Ekim 2026 dönemine düzeltme olarak işle" })
    .click();
  await expect(
    page.getByText("Talep geçmiş döneme düzeltme olarak işlendi", { exact: false }),
  ).toBeVisible();
  await expect(cards).toHaveCount(1);

  // 3) Seçilen aya uygula: varsayılan Kasım 2026, ay seçiciden Aralık.
  await cards
    .first()
    .getByRole("button", { name: "Seçilen aya uygula", exact: true })
    .click();
  const box = cards.first().getByTestId("change-request-target");
  const picker = box.getByRole("button", { name: "Hangi aya işlensin?" });
  await expect(picker).toContainText(/Kasım 2026/i);
  await picker.click();
  await page.getByRole("button", { name: "Ara", exact: true }).click();
  await expect(picker).toContainText(/Aralık 2026/i);
  await box.getByRole("button", { name: "Aralık 2026 ayına uygula" }).click();
  await expect(page.getByText("Talep Aralık 2026 ayına uygulandı.")).toBeVisible();

  // Kayıt: seçilen karar ve hedef ay.
  await expect.poll(async () => (await openList()).length).toBe(0);
  const all = (
    await api.json<{ items: ChangeRequest[] }>(
      await api.get(`${base}/change-requests?status=all`),
    )
  ).items;
  expect(all.map((c) => c.decision).sort()).toEqual([
    "correction",
    "reject",
    "target_period",
  ]);
  const moved = all.find((c) => c.decision === "target_period");
  expect(moved?.target_period).toBe(TARGET);
  expect(moved?.status).toBe("applied");
  expect(all.find((c) => c.decision === "reject")?.status).toBe("rejected");
  expect(all.find((c) => c.decision === "correction")?.target_period).toBeNull();

  // Fatura: seçilen aya (Aralık) yazıldı; düzeltme Ekim'de kalır (Go testi).
  const invoices = async (period: string) =>
    (
      await api.json<{ items: unknown[] }>(
        await api.get(`${base}/invoices?period=${period}&limit=50`),
      )
    ).items.length;
  expect(await invoices(TARGET)).toBe(1);

  // Liste: Geçmiş dönem takibi sekmesinde kararlar cümleyle.
  await page.goto(officePath(`/engagements/${engagement}/late-documents`));
  const table = page.getByTestId("change-request-decisions");
  await expect(table).toBeVisible();
  await expect(table.getByTestId("change-request-decision")).toHaveCount(3);
  await expect(table).toContainText("Aralık 2026 ayına uygulandı");
  await expect(table).toContainText("Ekim 2026 dönemine düzeltme olarak işlendi");
  await expect(table).toContainText("Reddedildi; kayıt değişmedi");
  await api.dispose();
});

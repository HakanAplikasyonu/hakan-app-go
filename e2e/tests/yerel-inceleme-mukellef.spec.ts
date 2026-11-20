import { expect, test } from "@playwright/test";

import { Api, DEMO, officePath, uiLogin } from "./lib";

/**
 * İnceleme kuyruğu mükellef bazında (#710): kuyruğun üstünde "Mükellefe
 * göre" seçimi; "Tümü" ve bekleyeni olan her mükellef sayısıyla. Sayılar
 * doğru: mükelleflerin toplamı kuyruğun toplamıdır, her mükellefin sayısı
 * kendi kuyruğunun toplamıdır. Mükellef seçilince kuyruk onun işleriyle
 * açılır ve sayaç aynı sayıyı yazar.
 *
 * Veri: demo ofisinin bekleyen önerileri. Dosya karar vermez.
 */
type Counts = { items: { engagement_uuid: string; total: number }[]; total: number };

test("kuyruk mükellefe göre seçilir; sayılar kuyrukla aynı (#710)", async ({ page }) => {
  test.setTimeout(120_000);
  const api = await Api.login(DEMO.owner);
  const counts = await api.json<Counts>(await api.get("/v1/tenant/review/taxpayers"));
  const queue = await api.json<{ total: number }>(await api.get("/v1/tenant/review/suggestions?status=pending&limit=1"));
  expect(counts.total).toBe(queue.total);
  expect(counts.items.reduce((n, x) => n + x.total, 0)).toBe(counts.total);
  expect(counts.items.length, "demo ofisinde bekleyen önerisi olan mükellef").toBeGreaterThan(0);
  for (const x of counts.items.slice(0, 3)) {
    const one = await api.json<{ total: number }>(
      await api.get(`/v1/tenant/engagements/${x.engagement_uuid}/suggestions?status=pending&limit=1`),
    );
    expect(one.total, x.engagement_uuid).toBe(x.total);
  }
  // Sıralama: çok olan önce.
  const totals = counts.items.map((x) => x.total);
  expect(totals).toEqual([...totals].sort((a, b) => b - a));
  expect((await api.get("/v1/tenant/review/taxpayers?status=bogus")).status()).toBe(400);
  await api.dispose();

  // Ekran: "Tümü" sayısı sayaçla aynı; mükellef seçilince kuyruk onun.
  await uiLogin(page, DEMO.owner);
  await page.goto(officePath("/review"));
  const group = page.getByRole("group", { name: "Mükellefe göre" });
  const all = group.getByRole("button", { name: /^Tümü · \d+$/ });
  await expect(all).toHaveAttribute("aria-pressed", "true");
  const allCount = Number((await all.innerText()).split("·")[1].trim());
  await expect(page.getByText(`${allCount} öneri bekliyor`)).toBeVisible();

  const first = group.getByRole("button").nth(1);
  const label = await first.innerText();
  const n = Number(label.split("·").pop()!.trim());
  expect(n).toBeGreaterThan(0);
  await first.click();
  await expect(first).toHaveAttribute("aria-pressed", "true");
  await expect(page).toHaveURL(/[?&]taxpayer=/);
  if (n <= 100) await expect(page.getByText(`${n} öneri bekliyor`)).toBeVisible();

  // "Tümü" kuyruğu geri açar.
  await all.click();
  await expect(all).toHaveAttribute("aria-pressed", "true");
  await expect(page).not.toHaveURL(/[?&]taxpayer=/);
});

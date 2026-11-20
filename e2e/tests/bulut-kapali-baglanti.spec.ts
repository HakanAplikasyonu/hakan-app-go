import { expect, test } from "@playwright/test";

import { Api, DEMO, engagementOf, officePath, uiLogin } from "./lib";

/**
 * Sunucuda yapılandırılmamış bağlantılar (#639): Paraşüt ve KolayBi liste
 * yanıtı `configured` alanını taşır. Kapalıysa Bağlantılar sekmesi
 * "Bu bağlantı sunucuda henüz etkin değil" boş durumunu gösterir, bağlan
 * düğmesi çıkmaz ve 503 dönen bir istek atılmaz. Açıksa bağlan eylemi
 * görünür. Veri: demo mükellefi; yalnız okur, kayıt eklemez.
 */
test("kapalı ön muhasebe bağlantısı boş durumla görünür, 503 isteği atılmaz", async ({ page }) => {
  test.setTimeout(120_000);
  const api = await Api.login(DEMO.owner);
  const engagement = await engagementOf(api, DEMO.atlas.title);
  const base = `/v1/tenant/engagements/${engagement}`;
  const parasut = await api.json<{ configured: boolean }>(await api.get(`${base}/parasut-connections`));
  const kolaybi = await api.json<{ configured: boolean }>(await api.get(`${base}/kolaybi-connections`));
  expect(typeof parasut.configured).toBe("boolean");
  expect(typeof kolaybi.configured).toBe("boolean");

  const failed: string[] = [];
  page.on("response", (res) => {
    if (/\/(parasut|kolaybi)-connections/.test(res.url()) && res.status() >= 400) {
      failed.push(`${res.status()} ${res.url()}`);
    }
  });

  await uiLogin(page, DEMO.owner);
  await page.goto(officePath(`/engagements/${engagement}/connections`));
  await expect(page.getByRole("heading", { name: "Paraşüt" })).toBeVisible({ timeout: 30_000 });
  await expect(page.getByRole("heading", { name: "KolayBi" })).toBeVisible();

  const disabled = page.getByText("Bu bağlantı sunucuda henüz etkin değil.");
  await expect(disabled).toHaveCount(Number(!parasut.configured) + Number(!kolaybi.configured));
  if (!parasut.configured) {
    await expect(page.getByRole("button", { name: "Paraşüt'e bağlan" })).toHaveCount(0);
  }
  await api.dispose();
  expect(failed, failed.join("\n")).toEqual([]);
});

import { randomUUID } from "node:crypto";

import { expect, request as pwRequest, test } from "@playwright/test";

import { API_URL, Api, DEMO, officePath, uiLogin } from "./lib";

type Item = {
  uuid: string;
  channel: "sms" | "email";
  to: string;
  status: string;
  provider_job_id: string;
  source_rule: string;
  delivered_at: string | null;
  opened_at: string | null;
  created_at: string;
};

/**
 * İletişim merkezi, iletişim geçmişi (#780 D): mükellef sayfasındaki
 * "İletişim geçmişi" sekmesi SMS ve e-postaları yeniden eskiye birlikte
 * gösterir; SMS'te NetGSM iş no ve iletildi zamanı, e-postada açıldı zamanı
 * kanıt olarak yazar. "E-posta açılmadı, SMS gitti" örneğinde not çıkar.
 * Kanal süzgeci yalnız o kanalı bırakır. İzleme pikseli oturumsuz çalışır,
 * bozuk anahtara da aynı resmi verir; ek bağlantısı yabancı adrese
 * yönlendirmez.
 *
 * Veri: demo ofisinin ilk üç mükellefindeki örnek geçmiş (salt okunur).
 */
test("iletişim geçmişi: SMS ve e-posta kanıtı, açılmadı → SMS örneği, süzgeç, piksel (#780)", async ({
  page,
}) => {
  const api = await Api.login(DEMO.owner);
  const engagements = await api.json<{ items: { uuid: string }[] }>(
    await api.get("/v1/tenant/engagements?limit=100"),
  );
  let proof: { engagement: string; items: Item[] } | undefined;
  let fallback: { engagement: string; items: Item[] } | undefined;
  for (const e of engagements.items) {
    const data = await api.json<{ items: Item[] }>(
      await api.get(`/v1/tenant/engagements/${e.uuid}/communications`),
    );
    const items = data.items;
    if (
      !fallback &&
      items.some(
        (m) => m.channel === "sms" && m.source_rule.includes("eposta_acilmadi"),
      )
    ) {
      fallback = { engagement: e.uuid, items };
    } else if (
      !proof &&
      items.some((m) => m.channel === "email" && m.opened_at) &&
      items.some((m) => m.channel === "sms" && m.delivered_at)
    ) {
      proof = { engagement: e.uuid, items };
    }
    if (proof && fallback) break;
  }
  expect(proof, "demo: iletildi / açıldı bilgili geçmiş").toBeTruthy();
  expect(fallback, "demo: e-posta açılmadı, SMS gitti örneği").toBeTruthy();

  // Yeniden eskiye sıralı.
  const times = proof!.items.map((m) => Date.parse(m.created_at));
  expect(times.length).toBeGreaterThanOrEqual(2);
  expect([...times].sort((a, b) => b - a)).toEqual(times);
  const sms = proof!.items.find((m) => m.channel === "sms" && m.delivered_at)!;
  expect(sms.provider_job_id).not.toBe("");

  // Kanal süzgeci API'de de çalışır; bilinmeyen kanal 400.
  const onlySms = await api.json<{ items: Item[] }>(
    await api.get(
      `/v1/tenant/engagements/${proof!.engagement}/communications?channel=sms`,
    ),
  );
  expect(onlySms.items.every((m) => m.channel === "sms")).toBe(true);
  const bad = await api.get(
    `/v1/tenant/engagements/${proof!.engagement}/communications?channel=fax`,
  );
  expect(bad.status()).toBe(400);

  await page.setViewportSize({ width: 1440, height: 900 });
  await uiLogin(page, DEMO.owner);
  await page.goto(officePath(`/engagements/${proof!.engagement}`));
  await page
    .getByTestId("engagement-tabs")
    .getByRole("link", { name: "İletişim geçmişi" })
    .click();
  await expect(page).toHaveURL(/\/communications$/);
  const list = page.getByTestId("communications-list");
  await expect(list.getByTestId("communication-item").first()).toBeVisible();
  await expect(
    list.locator('[data-outcome="delivered"]').first(),
  ).toBeVisible();
  await expect(list.getByText("NetGSM iş no").first()).toBeVisible();
  await expect(list.getByText(sms.provider_job_id).first()).toBeVisible();
  await expect(list.getByText("Telefona iletildi").first()).toBeVisible();
  await expect(
    list
      .locator('[data-channel="email"]')
      .getByText("Açıldı", { exact: true })
      .first(),
  ).toBeVisible();

  // Süzgeç: yalnız e-posta.
  await page.getByTestId("communications-filter-email").click();
  await expect(list.locator('[data-channel="sms"]')).toHaveCount(0);
  await expect(list.locator('[data-channel="email"]').first()).toBeVisible();

  // "E-posta açılmadı, SMS gitti" örneği.
  await page.goto(
    officePath(`/engagements/${fallback!.engagement}/communications`),
  );
  await expect(
    page.getByTestId("communication-fallback").first(),
  ).toBeVisible();
  await expect(page.locator('[data-outcome="unopened"]').first()).toBeVisible();
  await expect(page.getByText("Gönderildi, açılmadı").first()).toBeVisible();

  // İzleme pikseli: oturumsuz, bilinmeyen anahtara da aynı resim.
  const anon = await pwRequest.newContext({ baseURL: API_URL });
  const pixel = await anon.get(`/v1/public/mail/${randomUUID()}/open`);
  expect(pixel.status()).toBe(200);
  expect(pixel.headers()["content-type"]).toBe("image/gif");
  const garbage = await anon.get("/v1/public/mail/bozuk-anahtar/open");
  expect(garbage.status()).toBe(200);
  // Ek bağlantısı yabancı adrese yönlendirmez (açık yönlendirme yok).
  const evil = await anon.get(
    `/v1/public/mail/${randomUUID()}/link?to=${encodeURIComponent("https://kotu.example.com/")}`,
    { maxRedirects: 0 },
  );
  expect(evil.status()).toBe(404);
  // Uygulamanın kendi sayfasına yönlendirir (BFF yönlendirmeyi tarayıcıya bırakır).
  const own = await page.request.get(
    `/api/v1/public/mail/${randomUUID()}/link?to=${encodeURIComponent("/t/" + DEMO.org)}`,
    { maxRedirects: 0 },
  );
  expect(own.status()).toBe(302);
  expect(own.headers()["location"]).toContain(`/t/${DEMO.org}`);
  await anon.dispose();
});

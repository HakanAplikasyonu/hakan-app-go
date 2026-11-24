import { expect, test } from "@playwright/test";

import { Api, DEMO, engagementOf, officePath, uiLogin } from "./lib";

/**
 * İletişim sayfası (#780 B): mükellef grupları ve elle toplu gönderim.
 * Bir grup API ile kurulur; İletişim > Gruplar'da görünür, "Bu gruba yaz"
 * toplu gönderime götürür. E-posta sekmesinde metin yazılır; onay
 * penceresi "2 mükellefe gidecek" der; gönderim arka planda biter ("Bitti")
 * ve son gönderilenlerde görünür. 500'den fazla mükellef ve iletişim
 * bilgisi olmayan hedef reddedilir. Mükellef listesinde seçili satıra
 * "Toplu mesaj" açılır ve SMS boyu sayacı canlı sayar.
 *
 * Veri: demo ofisi (mükelleflerin kayıtlı e-postası var, telefonu yok);
 * deneme modu açık: mesajlar yalnız kaydedilir. Grup her koşuda yeni adla
 * kurulur ve sonda silinir.
 */
test("iletişim: grup, toplu e-posta onayı ve ilerleme, toplu mesaj sayacı (#780)", async ({
  page,
}) => {
  test.setTimeout(120_000);
  const api = await Api.login(DEMO.owner);
  const atlas = await engagementOf(api, DEMO.atlas.title);
  const selin = await engagementOf(api, DEMO.selin.title);
  const name = `E2E grup ${Date.now()}`;
  const group = await api.json<{ uuid: string; member_count: number }>(
    await api.post("/v1/tenant/communication/groups", {
      name,
      description: "e2e",
      engagement_uuids: [atlas, selin],
    }),
  );
  expect(group.member_count).toBe(2);
  // Aynı ad ikinci kez kaydedilemez.
  let res = await api.post("/v1/tenant/communication/groups", { name: name.toUpperCase() });
  expect(res.status()).toBe(400);

  // 500 sınırı ve iletişim bilgisi olmayan hedef.
  const many = Array.from({ length: 501 }, (_, i) => `00000000-0000-4000-8000-${String(i).padStart(12, "0")}`);
  res = await api.post("/v1/tenant/communication/bulk-sends/preview", {
    channel: "mail",
    subject: "Konu",
    body: "Metin",
    target: { kind: "selection", engagement_uuids: many },
  });
  expect(res.status()).toBe(400);
  expect(await res.text()).toContain("500");

  await page.setViewportSize({ width: 1440, height: 900 });
  await uiLogin(page, DEMO.owner);
  await page.goto(officePath("/communication?tab=groups"));
  const row = page.getByTestId("group-list").locator("li", { hasText: name });
  await expect(row).toBeVisible();
  await expect(row.getByText("2 mükellef")).toBeVisible();
  await row.getByRole("button", { name: "Bu gruba yaz" }).click();

  const composer = page.getByTestId("bulk-composer");
  await expect(composer).toBeVisible();
  await composer.getByRole("tab", { name: "E-posta" }).click();
  await composer.getByLabel("Konu").fill("E2E :donem bilgilendirme");
  await composer.getByLabel("Mesaj").fill("Sayın :mukellef, bu bir e2e toplu mesajıdır. :ofis");
  await page.getByTestId("bulk-preview").click();
  await expect(page.getByTestId("bulk-confirm-title")).toHaveText("2 mükellefe gidecek");
  await expect(page.getByTestId("bulk-duplicates")).toContainText("Tekrarlanan");
  await page.getByTestId("bulk-confirm").click();

  await expect(page.getByTestId("bulk-progress")).toBeVisible();
  await expect(page.getByTestId("bulk-status")).toHaveAttribute("data-status", "done", { timeout: 30_000 });
  await expect(page.getByTestId("bulk-counts")).toContainText("2 / 2 işlendi");

  // Son gönderilenler: kullanıcının en yeni gönderimi bu.
  const recent = await api.json<{ items: { target_label: string; total: number; sent: number; status: string }[] }>(
    await api.get("/v1/tenant/communication/bulk-sends"),
  );
  expect(recent.items.length).toBeLessThanOrEqual(5);
  expect(recent.items[0]).toMatchObject({ target_label: `Grup: ${name}`, total: 2, sent: 2, status: "done" });

  // Mükellef listesi: seçili satıra "Toplu mesaj", canlı SMS sayacı.
  await page.goto(officePath("/taxpayers"));
  // Liste sayfalı; diğer dosyaların eklediği mükelleflerle Atlas ilk sayfada
  // olmayabilir: unvanla aranır.
  await page.getByPlaceholder("Unvan ya da VKN ara").fill(DEMO.atlas.title);
  const tpRow = page.getByRole("row", { name: new RegExp(DEMO.atlas.title) });
  await tpRow.getByRole("checkbox").first().check();
  await page.getByRole("button", { name: "Toplu mesaj" }).click();
  const dialog = page.getByRole("dialog");
  await expect(dialog.getByText("Seçili 1 mükellefe toplu mesaj")).toBeVisible();
  await dialog.getByLabel("Mesaj").fill("a".repeat(150));
  await expect(dialog.getByTestId("sms-counter")).toContainText("155 karakter · 1 SMS");
  await dialog.getByLabel("Mesaj").fill(`${"a".repeat(149)}ş`);
  await expect(dialog.getByTestId("sms-counter")).toContainText("156 karakter · 2 SMS");
  await page.keyboard.press("Escape");

  res = await api.delete(`/v1/tenant/communication/groups/${group.uuid}`);
  expect(res.status()).toBe(204);
  await api.dispose();
});

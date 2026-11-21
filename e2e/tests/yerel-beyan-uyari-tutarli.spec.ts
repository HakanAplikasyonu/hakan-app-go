import { expect, test } from "@playwright/test";

import { Api, DEMO, engagementOf, officePath, uiLogin } from "./lib";

/**
 * Beyan ekranı uyarıları tek dille söyler (#669 UX geçişi): taslak beyanda
 * onay bekleyen uyarılar kontrol listesindedir. Durum kartı kaç maddenin
 * "Kontrol ettim" ile işaretlenmesi gerektiğini söyler ve tek tıkla kontrol
 * listesine götürür; madde kutusunun yazısı "Kontrol ettim"dir (eskiden
 * işaretsizken de "Onaylandı" yazıyordu). "Beyanı etkileyen N açık uyarı"
 * kutusu listelediği uyarıyı sayar; listelenecek uyarı yoksa hiç çıkmaz
 * ("3 açık uyarı" deyip altında "uyarı kalmadı" yazmak çelişkiydi).
 *
 * Veri: demo ofisi, Atlas Gıda, Ekim 2026 KDV1; yalnız okur ve gezinir.
 */
type Preview = { status: string; open_warnings: string[] };

test("beyan: onay bekleyen uyarılar kontrol listesine götürür, sayılar çelişmez", async ({ page }) => {
  const api = await Api.login(DEMO.owner);
  const engagement = await engagementOf(api, DEMO.atlas.title);
  const preview = await api.json<Preview>(
    await api.get(`/v1/tenant/engagements/${engagement}/tax-returns/kdv1/2026-10`),
  );
  await api.dispose();

  await uiLogin(page, DEMO.owner);
  await page.goto(officePath(`/engagements/${engagement}/tax-returns?period=2026-10`));
  const checklist = page.getByRole("region", { name: "Kontrol listesi" });
  await expect(checklist).toBeVisible();

  // Kutu varsa saydığı kadar uyarı listeler; "kalmadı" diyen boş kutu yok.
  await expect(page.getByText("Bu dönemde listelenecek açık uyarı kalmadı.")).toHaveCount(0);
  const blocking = page.getByRole("region", { name: /^Beyanı etkileyen \d+ açık uyarı$/ });
  if ((await blocking.count()) > 0) {
    await expect(blocking.getByTestId("tax-return-problems")).toBeVisible();
  }

  test.skip(
    preview.status !== "draft" || preview.open_warnings.length === 0,
    "Ekim 2026 KDV1'de onay bekleyen uyarı yok",
  );
  const open = preview.open_warnings.length;
  const status = page.getByRole("region", { name: "Beyan durumu" });
  await expect(status).toContainText(
    `Hazırlamak için kontrol listesindeki ${open} uyarıyı inceleyip "Kontrol ettim" ile işaretleyin.`,
  );
  await status.getByRole("link", { name: "Kontrol listesine git" }).click();
  await expect(page).toHaveURL(/#tr-checklist$/);
  await expect(checklist).toBeInViewport();
  await expect(checklist.getByRole("checkbox", { name: "Kontrol ettim" }).first()).toBeVisible();
  await expect(checklist.getByText("Onaylandı")).toHaveCount(0);
});

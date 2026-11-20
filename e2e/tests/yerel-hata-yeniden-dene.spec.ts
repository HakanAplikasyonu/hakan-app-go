import { expect, test } from "@playwright/test";

import { Api, DEMO, engagementOf, officePath, uiLogin } from "./lib";

/**
 * Yükleme hatası boşluk ya da "bulunamadı" gibi görünmez (#710, tasarım
 * PR-2): dönem matrisi yüklenemezse mükellef ayrıntısı ve Komuta merkezi
 * "Veriler yüklenemedi" der ve "Yeniden dene" sunar; sunucu düzelince
 * yeniden deneme sayfayı açar. Mükellef ayrıntısı artık bu durumda
 * "Mükellef bulunamadı" demez.
 *
 * Hata, tarayıcıda matris isteği 500'e çevrilerek kurulur; demo verisi
 * yalnızca okunur.
 */
const MATRIX = "**/api/v1/tenant/periods/matrix**";

test("matris yüklenemezse hata ve yeniden dene görünür; düzelince sayfa açılır (#710)", async ({ page }) => {
  test.setTimeout(120_000);
  const api = await Api.login(DEMO.owner);
  const atlas = await engagementOf(api, DEMO.atlas.title);
  await api.dispose();
  await uiLogin(page, DEMO.owner);

  let broken = true;
  await page.route(MATRIX, (route) =>
    broken
      ? route.fulfill({ status: 500, json: { success: false, error: { code: "INTERNAL", message: "test" } } })
      : route.fallback(),
  );

  // Mükellef ayrıntısı: yükleme hatası "bulunamadı" değildir.
  await page.goto(officePath(`/engagements/${atlas}`));
  const alert = page.getByRole("alert").filter({ hasText: "Veriler yüklenemedi" }).first();
  await expect(alert).toBeVisible({ timeout: 30_000 });
  await expect(page.getByText("Mükellef bulunamadı")).toHaveCount(0);
  broken = false;
  await alert.getByRole("button", { name: "Yeniden dene" }).click();
  await expect(page.getByRole("heading", { level: 1, name: DEMO.atlas.title })).toBeVisible();

  // Komuta merkezi: matris kartında hata ve yeniden dene.
  broken = true;
  await page.goto(officePath("/command"));
  const cmd = page.getByRole("alert").filter({ hasText: "Veriler yüklenemedi" }).first();
  await expect(cmd).toBeVisible({ timeout: 30_000 });
  broken = false;
  await cmd.getByRole("button", { name: "Yeniden dene" }).click();
  await expect(page.getByText(DEMO.atlas.title).first()).toBeVisible();
});

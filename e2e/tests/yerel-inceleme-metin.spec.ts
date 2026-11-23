import { expect, test } from "@playwright/test";

import { DEMO, officePath, uiLogin } from "./lib";

/**
 * İnceleme kuyruğu metinleri (#710): toplu onay satırı sayı sıfırken de ne
 * yapılacağını söyler; güven çubuğundaki çizginin ne olduğu yazar; "Geri al"
 * ipucunda "API" gibi kod sözcüğü yok.
 *
 * Veri: demo ofisi; yalnız okur, karar vermez.
 */
test("inceleme kuyruğu: toplu onay, güven çizgisi ve geri al sade Türkçe (#710)", async ({
  page,
}) => {
  await page.setViewportSize({ width: 1440, height: 900 });
  await uiLogin(page, DEMO.owner);
  await page.goto(officePath("/review"));

  // #740 sonra şeridi ikiye ayırdı: öneri varken "Bu sayfada …" (toplu onay
  // yalnız yüklü sayfayı kapsar), yokken ne yapılacağını söyleyen cümle.
  await expect(
    page.getByText(
      /Bu sayfada \d+ öneri güven %\d+ ve üzerinde\.|Toplu onay için güveni %\d+ ve üzerinde öneri yok\. Kuyruktaki \d+ öneriyi aşağıda tek tek inceleyebilir ya da eşiği düşürebilirsiniz\./,
    ),
  ).toBeVisible();
  await expect(
    page.getByText(/Güven · dikey çizgi %\d+ onay eşiğidir/),
  ).toBeVisible();

  await page.getByRole("button", { name: /Geri al/ }).hover();
  await expect(
    page.getByText(/satırdaki kod eski haline döner/).first(),
  ).toBeVisible();
  await expect(page.getByText(/API/)).toHaveCount(0);
});

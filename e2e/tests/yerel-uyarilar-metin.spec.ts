import { expect, test } from "@playwright/test";

import {
  Api,
  DEMO,
  addConsentedTaxpayer,
  officePath,
  uiLogin,
  uniqueVkn,
} from "./lib";

/**
 * Uyarılar, yeni müşavir gözüyle (#710): boş sekmeler ne olduğunu ve
 * uyarının oraya nasıl geldiğini söyler; uyarı metinlerinde "Schematron",
 * "ProfileID" gibi teknik adlar yok.
 *
 * Veri: kendi yeni mükellefi (görülmüş ya da çözülmüş uyarısı yok); ofis
 * listesi yalnız okunur.
 */
const run = Date.now().toString().slice(-6);
const taxpayer = { vkn: uniqueVkn(), title: `Uyarı Metin ${run} Ltd. Şti.` };

test("uyarılar: boş sekme yol gösterir, teknik ad yok (#710)", async ({
  browser,
  page,
}) => {
  test.setTimeout(240_000);
  const api = await Api.login(DEMO.owner);
  const engagement = await addConsentedTaxpayer(
    browser,
    api,
    taxpayer,
    "uyarimetin",
  );
  await api.dispose();

  await page.setViewportSize({ width: 1440, height: 900 });
  await uiLogin(page, DEMO.owner);

  await page.goto(
    officePath(`/engagements/${engagement}/warnings?status=acknowledged`),
  );
  await expect(
    page.getByText(/Açık sekmesinde "Gördüm" deyin; burada toplanır\./),
  ).toBeVisible();

  await page.goto(
    officePath(`/engagements/${engagement}/warnings?status=resolved`),
  );
  await expect(
    page.getByText(/"Çözüldü" dediğinizde uyarı buraya taşınır\./),
  ).toBeVisible();

  await page.goto(officePath("/warnings"));
  await expect(page.getByRole("heading", { name: "Uyarılar" })).toBeVisible();
  await expect(page.getByText(/Schematron|ProfileID/)).toHaveCount(0);
});

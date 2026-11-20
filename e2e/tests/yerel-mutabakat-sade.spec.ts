import { expect, test } from "@playwright/test";

import { Api, DEMO, engagementOf, officePath, uiLogin } from "./lib";

/**
 * Mutabakat ekranının sade anlatımı (#710): "Bu ekran ne yapar?" kutusu
 * mizanı, uygulamadaki kayıtları ve farkı tek cümleyle anlatır; fark
 * tablosunda her hesabın "Ne demek?" cümlesi var (fark tutarı, hangi taraf
 * fazla ya da hesap hangi tarafta yok). Eşleşen hesap "Tutuyor.".
 *
 * Veri: demo ofisi, Atlas Gıda'nın demo mutabakatı; yalnız okunur.
 */
test("mutabakat: sade anlatım ve her farkın cümlesi", async ({ page }) => {
  const api = await Api.login(DEMO.owner);
  const engagement = await engagementOf(api, DEMO.atlas.title);
  await api.dispose();

  await uiLogin(page, DEMO.owner);
  await page.goto(officePath(`/engagements/${engagement}/reconciliation`));
  const box = page.getByRole("region", { name: "Bu ekran ne yapar?" });
  await expect(box).toBeVisible();
  await expect(box).toContainText("hesap hesap karşılaştırır");
  await expect(box).toContainText("Mizan − uygulama. Sıfırsa hesap tutuyor.");

  await expect(
    page.getByRole("columnheader", { name: "Ne demek?" }),
  ).toBeVisible();
  const meanings = page.getByTestId("reconciliation-meaning");
  // Açık farklar: her satırda tutarlı bir cümle.
  await expect(meanings.first()).toBeVisible();
  const open = await meanings.allInnerTexts();
  expect(open.length).toBeGreaterThan(0);
  for (const text of open)
    expect(text).toMatch(
      /(₺ [\d.]+,\d{2} fark: mizanda (borç|alacak) tarafı fazla|mizanda yok|bu hesaba kayıt yok)/,
    );

  // Eşleşenler de görünür olunca "Tutuyor." cümlesi gelir.
  await page.getByRole("checkbox", { name: "Yalnızca açık farklar" }).click();
  await expect(
    meanings.filter({ hasText: /^Tutuyor\.$/ }).first(),
  ).toBeVisible();
});

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
 * Mükellef özeti, yeni müşavir gözüyle (#710): belge isteği yoksa ne işe
 * yaradığını ve isteğin nereden açıldığını söyler, tek tıkla uyarılara
 * götürür. İstekler yüklenemezse "0" yazıp yanıltmaz; hata ve "Yeniden dene"
 * görünür.
 *
 * Veri: kendi yeni mükellefi (belge isteği yok).
 */
const run = Date.now().toString().slice(-6);
const taxpayer = { vkn: uniqueVkn(), title: `Özet Durum ${run} Ltd. Şti.` };

test("mükellef özeti: boş belge isteği eylem gösterir, hata 0 yazmaz (#710)", async ({
  browser,
  page,
}) => {
  test.setTimeout(240_000);
  const api = await Api.login(DEMO.owner);
  const engagement = await addConsentedTaxpayer(
    browser,
    api,
    taxpayer,
    "ozetdurum",
  );
  await api.dispose();

  await page.setViewportSize({ width: 1440, height: 900 });
  await uiLogin(page, DEMO.owner);

  // Hata: istekler gelmezse sayaç "–", bölümde hata ve yeniden dene.
  await page.route("**/document-tasks?**", (r) => r.abort());
  await page.goto(officePath(`/engagements/${engagement}`));
  const tasks = page.getByRole("region", { name: "Açık belge istekleri" });
  await expect(tasks.getByText("Belge istekleri yüklenemedi")).toBeVisible({
    timeout: 30_000,
  });
  await expect(
    page.getByRole("link", { name: /Açık belge isteği/ }),
  ).toContainText("–");

  // Yeniden dene: boş durum ne işe yaradığını söyler ve uyarılara götürür.
  await page.unroute("**/document-tasks?**");
  await tasks.getByRole("button", { name: "Yeniden dene" }).click();
  await expect(tasks.getByText("Açık belge isteği yok")).toBeVisible();
  await expect(
    tasks.getByText(/Uyarılarda eksik belge için mükelleften istediğinizde/),
  ).toBeVisible();
  await tasks.getByRole("link", { name: "Uyarılara git" }).click();
  await expect(page).toHaveURL(
    new RegExp(`/engagements/${engagement}/warnings`),
  );
});

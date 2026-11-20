import { expect, test } from "@playwright/test";

import { addConsentedTaxpayer, Api, DEMO, officePath, uiLogin, uniqueVkn } from "./lib";

/**
 * Bordro personelini elle ekleme ve düzenleme (#669 J): Personel görünümünde
 * "Personel ekle" ile yeni kişi girilir, listede ve API'de görünür; kalem
 * düğmesiyle görevi değiştirilir, değişiklik listede ve API'de görünür.
 *
 * Veri: demo ofisine her koşuda yeni bir mükellef; personel arayüzden girilir.
 */
test.describe.configure({ mode: "serial" });

const run = `${Date.now()}`.slice(-6);
const taxpayer = { vkn: uniqueVkn(), title: `Bordro Personel ${run} A.Ş.` };
const NAME = `Ayşe Deneme ${run}`;

type Employee = { uuid: string; full_name: string; sgk_no: string | null; job_title: string | null; status: string };

test.describe("bordro personeli elle girilir (#669 J)", () => {
  let engagement = "";
  let api: Api;

  const employees = async () =>
    (
      await api.json<{ items: Employee[] }>(
        await api.get(`/v1/tenant/engagements/${engagement}/payroll/employees?status=all&q=${encodeURIComponent(NAME)}&limit=10`),
      )
    ).items;

  test.beforeAll(async ({ browser }) => {
    test.setTimeout(180_000);
    api = await Api.login(DEMO.owner);
    engagement = await addConsentedTaxpayer(browser, api, taxpayer, "bordro");
  });

  test.afterAll(async () => {
    await api?.dispose();
  });

  test("Personel ekle, sonra görevini düzenle", async ({ page }) => {
    await uiLogin(page, DEMO.owner);
    await page.goto(officePath(`/engagements/${engagement}/payroll`));
    await page.getByRole("button", { name: "Personel", exact: true }).click();

    // Ekle.
    await page.getByRole("button", { name: "Personel ekle" }).click();
    const dialog = page.getByRole("dialog");
    await expect(dialog).toBeVisible();
    await dialog.getByLabel("Adı soyadı").fill(NAME);
    await dialog.getByLabel("SGK sicil no").fill("1234567");
    await dialog.getByLabel("Görevi").fill("Muhasebe elemanı");
    await dialog.getByRole("button", { name: "Kaydet" }).click();
    await expect(dialog).toBeHidden();

    const row = page.getByRole("row").filter({ hasText: NAME });
    await expect(row).toContainText("Muhasebe elemanı");
    await expect.poll(async () => (await employees()).length).toBe(1);
    const added = (await employees())[0];
    expect(added).toMatchObject({ full_name: NAME, sgk_no: "1234567", job_title: "Muhasebe elemanı", status: "active" });

    // Düzenle: görev değişir, diğer alanlar yerinde kalır.
    await row.getByRole("button", { name: `${NAME} bilgilerini düzenle` }).click();
    await expect(dialog.getByLabel("Görevi")).toHaveValue("Muhasebe elemanı");
    await dialog.getByLabel("Görevi").fill("Muhasebe sorumlusu");
    await dialog.getByRole("button", { name: "Kaydet" }).click();
    await expect(dialog).toBeHidden();

    await expect(row).toContainText("Muhasebe sorumlusu");
    await expect.poll(async () => (await employees())[0]?.job_title).toBe("Muhasebe sorumlusu");
    const edited = (await employees())[0];
    expect(edited).toMatchObject({ uuid: added.uuid, full_name: NAME, sgk_no: "1234567" });
  });
});

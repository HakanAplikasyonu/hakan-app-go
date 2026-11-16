import { expect, test } from "@playwright/test";

import { Api, DEMO, engagementOf, mailToken, registerWithToken, uniqueEmail } from "./lib";

/**
 * Mükellef paneli daveti (#181): ofis, demo ofisindeki Selin Yılmaz için bir
 * çalışanı mükellef paneline davet eder; davetli e-postadaki bağlantıyla
 * hesap açar, daveti kabul eder ve yalnızca o mükellefi görür.
 *
 * Ofis panelinde davet ekranı yok: davet API'den oluşturulur, kabul arayüzden.
 * Yalnızca Selin'e yeni bir üye eklediği için diğer dosyalarla paralel koşar.
 */
test("mükellef daveti: e-posta, kayıt, kabul ve yalnızca kendi şirketi", async ({ page }) => {
  const email = uniqueEmail("calisan");
  const password = "E2e-Parola-123!";

  const office = await Api.login(DEMO.owner);
  let selinTaxpayer: string;
  let atlasTaxpayer: string;
  try {
    const selin = await engagementOf(office, DEMO.selin.title);
    const atlas = await engagementOf(office, DEMO.atlas.title);
    selinTaxpayer = (await office.json<{ uuid: string }>(await office.get(`/v1/tenant/engagements/${selin}/taxpayer`))).uuid;
    atlasTaxpayer = (await office.json<{ uuid: string }>(await office.get(`/v1/tenant/engagements/${atlas}/taxpayer`))).uuid;
    await office.json(
      await office.post(`/v1/tenant/engagements/${selin}/invitations`, { email, role: "employee" }),
    );
  } finally {
    await office.dispose();
  }

  const token = await mailToken(email, "/invite/m/");
  await registerWithToken(page, token, `/invite/m/${token}`, { name: "Cem", surname: "Çalışan", password });
  await expect(page.getByText(DEMO.selin.title).first()).toBeVisible();
  await page.getByRole("button", { name: "Daveti kabul et" }).click();
  await page.waitForURL(new RegExp(`/m/${selinTaxpayer}`));
  await expect(page.getByText(DEMO.selin.title).first()).toBeVisible();

  // Çalışan rolüyle girdi; başka mükellefin verisi kapalı.
  const member = await Api.login(email, password);
  try {
    expect((await member.get("/v1/taxpayer/invoices", { "X-Taxpayer": selinTaxpayer })).status()).toBe(200);
    const other = await member.get("/v1/taxpayer/invoices", { "X-Taxpayer": atlasTaxpayer });
    expect(other.status()).toBe(403);
    expect((await other.json()).error.code).toBe("NO_TAXPAYER_MEMBERSHIP");
  } finally {
    await member.dispose();
  }
});

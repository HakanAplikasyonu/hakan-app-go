import { expect, test } from "@playwright/test";

import { addConsentedTaxpayerWithUser, Api, DEMO, officePath, uiLogin, uniqueVkn } from "./lib";

/**
 * Mükellef iletişim bilgisi (#780, Oğuz: "ne mailini görebiliyorum ne de
 * telefon numarasını"): ofis mükellef sayfasındaki "İletişim bilgileri"
 * bölümünde telefonu ve e-postayı düzenler; geçersiz telefon alanın altında
 * Türkçe hata verir, geçerli numara 0532 123 45 67 biçimine getirilir.
 * Yetkili kişi (muhasebe muhatabı: ad, telefon, e-posta) aynı formda girilir;
 * boşken bölüm mükellef hesabının sahibini "Hesap sahibi" olarak gösterir.
 * Mükellef kendi iletişim bilgisini /v1/taxpayer/context'te görür; etkinlik
 * kaydına değişiklik düşer.
 *
 * Veri: her koşuda yeni mükellef ve kullanıcısı (uydurma bilgiler).
 */
test.describe.configure({ mode: "serial" });

const run = `${Date.now()}`.slice(-6);
const taxpayer = { vkn: uniqueVkn(), title: `İletişim Deneme ${run} Ltd. Şti.` };
const EMAIL = `iletisim-${run}@ornek-firma.test`;
const PERSON_EMAIL = `yetkili-${run}@ornek-firma.test`;

let office: Api;
let engagement = "";
let taxpayerUuid = "";
let user = { email: "", password: "" };

test.beforeAll(async ({ browser }) => {
  test.setTimeout(240_000);
  office = await Api.login(DEMO.owner);
  const made = await addConsentedTaxpayerWithUser(browser, office, taxpayer, "iletisim");
  engagement = made.engagement;
  taxpayerUuid = made.taxpayerUuid;
  user = { email: made.email, password: made.password };
});

test.afterAll(async () => {
  await office?.dispose();
});

test("ofis telefonu ve e-postayı düzenler", async ({ page }) => {
  await uiLogin(page, DEMO.owner);
  await page.goto(officePath(`/engagements/${engagement}`));
  const section = page.getByRole("region", { name: "İletişim bilgileri" });
  await expect(section).toContainText("Hesap sahibi");
  await expect(section).toContainText(user.email);

  await section.getByRole("button", { name: "Düzenle" }).click();
  await section.getByLabel("Telefon", { exact: true }).fill("0632 12");
  await section.getByLabel("E-posta", { exact: true }).fill(EMAIL);
  await section.getByRole("button", { name: "Kaydet" }).click();
  await expect(section.getByRole("alert")).toContainText("Telefon numarası geçersiz");

  await section.getByLabel("Telefon", { exact: true }).fill("+90 (532) 765 43 21");
  await section.getByRole("button", { name: "Kaydet" }).click();
  await expect(section.getByRole("link", { name: "0532 765 43 21" })).toBeVisible();
  await expect(section.getByRole("link", { name: EMAIL })).toBeVisible();
});

test("mükellef kendi iletişim bilgisini görür", async () => {
  const tpApi = await Api.login(user.email, user.password);
  try {
    const res = await tpApi.get("/v1/taxpayer/context", { "X-Taxpayer": taxpayerUuid });
    expect(res.status(), await res.text()).toBe(200);
    const data = await tpApi.json<{ contact: { phone: string | null; email: string | null } }>(res);
    expect(data.contact).toEqual({ phone: "0532 765 43 21", email: EMAIL });
  } finally {
    await tpApi.dispose();
  }
});

test("geçersiz e-posta 400 ve Türkçe alan kodu döner", async () => {
  const res = await office.put(`/v1/tenant/engagements/${engagement}/contacts`, { email: "yanlis" });
  expect(res.status()).toBe(400);
  const body = await res.json();
  expect(body.error.details[0].code).toBe("eposta_gecersiz");
});

test("ofis yetkili kişiyi girer, siler; boşken hesap sahibi görünür", async ({ page }) => {
  await uiLogin(page, DEMO.owner);
  await page.goto(officePath(`/engagements/${engagement}`));
  const section = page.getByRole("region", { name: "İletişim bilgileri" });

  await section.getByRole("button", { name: "Düzenle" }).click();
  await section.getByLabel("Yetkili kişinin adı").fill(`  Deniz   Örnek ${run} `);
  await section.getByLabel("Yetkili kişinin telefonu").fill("12");
  await section.getByLabel("Yetkili kişinin e-postası").fill(PERSON_EMAIL);
  await section.getByRole("button", { name: "Kaydet" }).click();
  await expect(section.getByRole("alert")).toContainText("Telefon numarası geçersiz");

  await section.getByLabel("Yetkili kişinin telefonu").fill("0530 111 22 33");
  await section.getByRole("button", { name: "Kaydet" }).click();
  const person = section.getByTestId("contact-person");
  await expect(person).toContainText(`Deniz Örnek ${run}`);
  await expect(person.getByRole("link", { name: "0530 111 22 33" })).toHaveAttribute("href", "tel:05301112233");
  await expect(person.getByRole("link", { name: PERSON_EMAIL })).toHaveAttribute("href", `mailto:${PERSON_EMAIL}`);
  await expect(section).toContainText("Hesap sahibi");

  // Yalnız telefon silinir; ad ve e-posta (gönderilmedi) kalır; denetim kaydına eski→yeni düşer.
  const res = await office.put(`/v1/tenant/engagements/${engagement}/contacts`, { person_phone: "" });
  expect(res.status(), await res.text()).toBe(200);
  const body = await res.json();
  expect(body.data.person).toEqual({ name: `Deniz Örnek ${run}`, phone: null, email: PERSON_EMAIL });

  // Hepsi silinince bölüm yalnız hesap sahibini gösterir.
  const cleared = await office.put(`/v1/tenant/engagements/${engagement}/contacts`, { person_name: "", person_email: "" });
  expect(cleared.status()).toBe(200);
  expect((await cleared.json()).data.person).toBeNull();
  await page.reload();
  await expect(section).toContainText("Hesap sahibi");
  await expect(section.getByTestId("contact-person")).toHaveCount(0);
});

test("çok uzun yetkili kişi adı 400 ve Türkçe alan kodu döner", async () => {
  const res = await office.put(`/v1/tenant/engagements/${engagement}/contacts`, { person_name: "a".repeat(121) });
  expect(res.status()).toBe(400);
  const body = await res.json();
  expect(body.error.details[0]).toMatchObject({ field: "person_name", code: "ad_gecersiz" });
});

import { expect, test } from "@playwright/test";

import { Api, DEMO, officePath, uiLogin } from "./lib";

/**
 * İletişim merkezi, ilk parça (#780 A): Ayarlar > SMS ofisin kendi NetGSM
 * hesabını tutar. Şifre kaydedilir ama ekrana ve API cevabına hiç dönmez
 * ("Şifre kayıtlı"); boş bırakmak "değiştirmedim" demektir. Deneme modunda
 * deneme SMS'i yalnız kaydedilir ("Deneme modu, gönderilmedi"); sabit hat
 * baştan başarısız yazılır; kampanya dili reddedilir.
 *
 * Veri: demo ofisi, deneme modu hep açık. "Bağlantıyı test et" tıklanmaz:
 * testlerde NetGSM'e hiçbir istek gitmez.
 */
test("iletişim: NetGSM ayarı, şifre geri dönmez, deneme modunda SMS yalnız kaydedilir (#780)", async ({
  page,
}) => {
  const api = await Api.login(DEMO.owner);
  const base = {
    enabled: true,
    test_mode: true,
    usercode: "8500000000",
    msgheader: "OFISDEMO",
    encoding: "TR",
    appname: "Hakan",
  };
  // Deneme modunu garanti et (şifresiz kaydedilen ayar açılamaz; önce kapalı).
  let res = await api.put("/v1/tenant/sms-settings", { ...base, enabled: false });
  expect(res.status(), await res.text()).toBe(200);

  await page.setViewportSize({ width: 1440, height: 900 });
  await uiLogin(page, DEMO.owner);
  await page.goto(officePath("/settings#sms"));
  const card = page.getByTestId("sms-settings");
  await expect(card).toBeVisible();
  await expect(card.getByText("Deneme modu", { exact: true }).first()).toBeVisible();

  const secret = `e2e-gizli-${Date.now()}`;
  await card.getByLabel("API alt kullanıcı şifresi").fill(secret);
  await card.getByRole("switch", { name: "SMS gönderimi açık" }).click();
  await card.getByRole("button", { name: "Kaydet" }).click();
  await expect(page.getByText("SMS ayarları kaydedildi.")).toBeVisible();
  await expect(card.getByText("Şifre kayıtlı", { exact: true })).toBeVisible();
  await expect(card.getByLabel("API alt kullanıcı şifresi")).toHaveValue("");

  // Şifre API cevabında yok; yalnız password_set.
  res = await api.get("/v1/tenant/sms-settings");
  const raw = await res.text();
  expect(raw).not.toContain(secret);
  const settings = JSON.parse(raw).data;
  expect(settings.password_set).toBe(true);
  expect(settings.test_mode).toBe(true);
  expect(settings.enabled).toBe(true);

  // Boş şifreyle kaydetmek mevcut şifreyi korur.
  res = await api.put("/v1/tenant/sms-settings", { ...base, password: "" });
  expect((await res.json()).data.password_set).toBe(true);

  // Deneme SMS'i: deneme modunda yalnız kaydedilir.
  await page.getByLabel("Deneme SMS'i", { exact: true }).fill("0532 000 00 99");
  await page.getByRole("button", { name: "Deneme SMS'i gönder" }).click();
  await expect(page.getByText("Deneme modu: mesaj kaydedildi, gönderilmedi.")).toBeVisible();
  const recent = page.getByTestId("sms-recent");
  await expect(recent.getByText("5320000099").first()).toBeVisible();
  await expect(recent.getByText("Deneme modu, gönderilmedi", { exact: true }).first()).toBeVisible();

  // Sabit hat baştan başarısız; kampanya dili reddedilir.
  res = await api.post("/v1/tenant/sms-messages/test", { phone: "0212 000 00 00", text: "Bilgi" });
  expect(res.status()).toBe(200);
  const failed = (await res.json()).data;
  expect(failed.status).toBe("failed");
  expect(failed.error_message).toContain("cep telefonu");
  res = await api.post("/v1/tenant/sms-messages/test", { phone: "05320000099", text: "Bu ay %20 indirim kampanyası" });
  expect(res.status()).toBe(400);
  expect(await res.text()).toContain("kampanya");

  // Kayıtların hiçbiri gönderilmiş değil (NetGSM'e istek gitmedi).
  res = await api.get("/v1/tenant/sms-messages?limit=50");
  const items = (await res.json()).data.items as { status: string }[];
  expect(items.some((m) => m.status === "sent" || m.status === "uncertain")).toBe(false);
});

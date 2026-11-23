import { expect, test, type Route } from "@playwright/test";

import { Api, DEMO, engagementOf, officePath, uiLogin } from "./lib";

/**
 * Easytrade bağlantı formu hataları (#830 devamı): Easytrade girişi
 * reddedince form Easytrade'in kendi nedenini, yanıt veren sunucuyu ve HTTP
 * durumunu gösterir; 401 "E-posta veya şifre hatalı.", 422 alan adıyla
 * ("Şifre: …") ve ilgili alan işaretlenir. ID boşken test, Easytrade'in
 * söylediği firmayı "Bağlanılan firma: … (ID …)" diye yazar. Şifre alanı
 * tarayıcının kayıtlı Hakan şifresini doldurmaz (new-password).
 *
 * Veri: demo ofisi, Atlas Gıda. Bağlantı listesi boş, test ucu sahte
 * yanıtla kesilir; Easytrade'e gidilmez, hiçbir şey kaydedilmez.
 */
test("Easytrade formu: 401 / 422 nedeni, sunucu ve ID'siz test (#830)", async ({
  page,
}) => {
  await page.setViewportSize({ width: 1440, height: 900 });
  const api = await Api.login(DEMO.owner);
  const engagement = await engagementOf(api, DEMO.atlas.title);
  await api.dispose();

  const sent: Record<string, unknown>[] = [];
  const fail = (
    status: number,
    code: string,
    message: string,
    extra: { field: string; message: string; code: string }[] = [],
  ) => ({
    status: status === 401 || status === 422 ? 422 : 502,
    contentType: "application/json",
    body: JSON.stringify({
      success: false,
      error: {
        code,
        message,
        details: [
          {
            field: "easytrade_host",
            message: "pro.easytradetr.com",
            code: "EASYTRADE_HOST",
          },
          {
            field: "easytrade_status",
            message: String(status),
            code: "EASYTRADE_STATUS",
          },
          ...extra,
        ],
      },
      meta: { request_id: "e2e" },
    }),
  });
  let reply: (r: Route) => Promise<void> = (r) =>
    r.fulfill(
      fail(
        401,
        "EASYTRADE_BAD_CREDENTIALS",
        "Easytrade (pro.easytradetr.com) 401: E-posta veya şifre hatalı.",
      ),
    );

  await page.route(/\/easytrade-connections(\?.*)?$/, (r) =>
    r.request().method() === "GET"
      ? r.fulfill({
          status: 200,
          contentType: "application/json",
          body: JSON.stringify({
            success: true,
            data: { items: [] },
            meta: { request_id: "e2e" },
          }),
        })
      : r.fallback(),
  );
  await page.route(/\/easytrade-connections\/test$/, (r) => {
    sent.push(r.request().postDataJSON() as Record<string, unknown>);
    return reply(r);
  });

  await uiLogin(page, DEMO.owner);
  await page.goto(officePath(`/engagements/${engagement}/easytrade`));

  const email = page.getByLabel("Easytrade e-postası");
  const password = page.getByLabel("Şifre", { exact: true });
  await expect(email).toBeVisible();
  await expect(
    page.getByText(
      "Easytrade'e pro.easytradetr.com'da giriş yaptığınız e-posta ve şifre.",
    ),
  ).toBeVisible();
  await expect(page.getByLabel("ID (isteğe bağlı)")).toBeVisible();
  // Tarayıcı kullanıcının Hakan şifresini buraya doldurmasın.
  await expect(password).toHaveAttribute("autocomplete", "new-password");
  await expect(password).toHaveAttribute("name", "easytrade_password");
  await expect(email).toHaveAttribute("autocomplete", "off");
  await expect(email).toHaveAttribute("spellcheck", "false");

  await email.fill("  Muhasebe@Ornek.COM ");
  await password.fill(" gizli şifre ");
  const testButton = page.getByRole("button", { name: "Bağlantıyı test et" });

  // 401: Easytrade'in mesajı, sunucusu ve durumu.
  await testButton.click();
  const outcome = page.getByTestId("et-outcome");
  await expect(outcome).toContainText(
    "Easytrade (pro.easytradetr.com) 401: E-posta veya şifre hatalı.",
  );
  await expect(outcome).toContainText("EASYTRADE_BAD_CREDENTIALS");
  await expect(email).toHaveAttribute("aria-invalid", "true");
  // E-posta kırpılıp küçük harfle, şifre olduğu gibi gider.
  expect(sent[0]).toMatchObject({
    email: "muhasebe@ornek.com",
    password: " gizli şifre ",
    company_id: "",
  });

  // 422: alan adıyla Easytrade'in metni; şifre alanı işaretlenir.
  reply = (r) =>
    r.fulfill(
      fail(
        422,
        "EASYTRADE_INVALID_INPUT",
        "Easytrade (pro.easytradetr.com) 422: Şifre: Şifre en az 4 karakter olmalıdır.",
        [
          {
            field: "password",
            message: "Şifre: Şifre en az 4 karakter olmalıdır.",
            code: "EASYTRADE_FIELD",
          },
        ],
      ),
    );
  await password.fill("x");
  await testButton.click();
  await expect(outcome).toContainText(
    "Easytrade (pro.easytradetr.com) 422: Şifre: Şifre en az 4 karakter olmalıdır.",
  );
  await expect(page.getByTestId("et-password-hint")).toHaveText(
    "Şifre: Şifre en az 4 karakter olmalıdır.",
  );
  await expect(password).toHaveAttribute("aria-invalid", "true");

  // ID boş: Easytrade'in söylediği firma ekranda.
  reply = (r) =>
    r.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify({
        success: true,
        data: {
          ok: true,
          company_id: "123",
          company_name: "Atlas Gıda",
          branches: [{ id: "1", name: "Merkez" }],
        },
        meta: { request_id: "e2e" },
      }),
    });
  await password.fill("gizli");
  await testButton.click();
  await expect(outcome).toContainText(
    "Bağlanılan firma: Atlas Gıda (ID 123) · 1 şube.",
  );
});

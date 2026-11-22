import { expect, test } from "@playwright/test";

import {
  addConsentedTaxpayer,
  Api,
  DEMO,
  officePath,
  uiLogin,
  uniqueVkn,
} from "./lib";

/**
 * EDM e-fatura entegrasyonu ve bağlantı testi (#780).
 *
 * - Bağlantılar sekmesinde "Entegratör bağla" listesinde EDM Bilişim var;
 *   test ortamıyla açılınca web servis kullanıcı adı ve şifresi istenir.
 * - "Bağlantıyı test et" girilen (kaydedilmemiş) uydurma kullanıcıyla
 *   entegratörü bir kez çağırır ve sonucu Türkçe gösterir: EDM test
 *   sistemine ulaşılırsa "Kullanıcı adı veya şifre yanlış", ulaşılamazsa
 *   "servis yanıt vermiyor" ya da "zaman aşımı"; her durumda kısa bir
 *   teknik kod ve İngilizce/ham hata metni yok.
 * - Uç (`POST …/einvoice-connections/{uuid}/test`) entegratörün reddini
 *   200 `ok: false` olarak döner; kayıtlı kullanıcı yokken ve gövde boşken
 *   409 `INTEGRATOR_CONNECTION_STATE`.
 *
 * Veri: her koşuda yeni mükellef; gerçek EDM kullanıcısı kullanılmaz.
 */
test.describe.configure({ mode: "serial" });

const run = `${Date.now()}`.slice(-6);
const taxpayer = { vkn: uniqueVkn(), title: `EDM Entegrasyon ${run} A.Ş.` };
const TURKISH =
  /Kullanıcı adı veya şifre yanlış|servisi yanıt vermiyor|zaman aşımı|web servis yetkisi yok|Bilinmeyen bir hata/;

type Conn = {
  uuid: string;
  integrator_id: string;
  state: string;
  fields: { key: string }[];
};
type TestResult = { ok: boolean; kind?: string; code?: string };

test.describe("EDM e-fatura bağlantısı (#780)", () => {
  let engagement = "";
  let api: Api;
  const base = () =>
    `/v1/tenant/engagements/${engagement}/einvoice-connections`;

  test.beforeAll(async ({ browser }) => {
    test.setTimeout(240_000);
    api = await Api.login(DEMO.owner);
    engagement = await addConsentedTaxpayer(browser, api, taxpayer, "edm");
  });

  test.afterAll(async () => {
    await api?.dispose();
  });

  test("ekranda form ve test butonu Türkçe hata gösterir", async ({ page }) => {
    test.setTimeout(180_000);
    await uiLogin(page, DEMO.owner);
    await page.goto(officePath(`/engagements/${engagement}/connections`));
    await page
      .getByRole("button", { name: "Entegratör bağla" })
      .first()
      .click();
    const dialog = page.getByRole("dialog");
    await dialog.locator("#einvoice-id").click();
    await page.getByRole("option", { name: "EDM Bilişim" }).click();
    await dialog.locator("#einvoice-env").click();
    await page.getByRole("option", { name: "Test ortamı" }).click();
    await dialog.getByRole("button", { name: "Devam" }).click();
    await expect(dialog).toBeHidden();
    const card = page.locator("article").filter({ hasText: "EDM Bilişim" });
    await expect(card.getByText("Test ortamı")).toBeVisible();
    await card.getByLabel("Web servis kullanıcı adı").fill(`e2e-yok-${run}`);
    await card.getByLabel("Web servis şifresi").fill("uydurma-sifre");
    await card.getByRole("button", { name: "Bağlantıyı test et" }).click();

    const result = card.getByTestId("einvoice-test-result");
    await expect(result).toBeVisible({ timeout: 90_000 });
    await expect(result).toHaveText(TURKISH);
    await expect(result).toContainText("Teknik kod:");
    await expect(result).not.toContainText(/provider|soap fault/i);
  });

  test("katalogda EDM var; test ucu reddi 200 ok:false, boş gövde 409", async () => {
    test.setTimeout(120_000);
    const cat = await api.json<{ items: { id: string; name: string }[] }>(
      await api.get("/v1/tenant/einvoice-integrators"),
    );
    expect(cat.items.find((i) => i.id === "edm")?.name).toBe("EDM Bilişim");

    // Ekran testinde açılan bağlantı (kullanıcı kaydedilmedi: taslak).
    const list = await api.json<{ items: Conn[] }>(await api.get(base()));
    const conn = list.items.find((c) => c.integrator_id === "edm")!;
    expect(conn.state).toBe("draft");
    expect(conn.fields.map((f) => f.key)).toEqual(["username", "password"]);

    const empty = await api.post(`${base()}/${conn.uuid}/test`, {});
    expect(empty.status()).toBe(409);

    const res = await api.json<TestResult>(
      await api.post(`${base()}/${conn.uuid}/test`, {
        values: { username: `e2e-yok-${run}`, password: "uydurma-sifre" },
      }),
    );
    expect(res.ok).toBe(false);
    expect([
      "auth",
      "permission",
      "timeout",
      "unavailable",
      "unknown",
    ]).toContain(res.kind);
    expect(res.code ?? "").not.toBe("");
  });
});

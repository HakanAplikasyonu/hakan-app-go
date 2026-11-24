import { expect, test } from "@playwright/test";

import { Api, DEMO, engagementOf, officePath, uiLogin } from "./lib";

/**
 * Belgeler → Dilekçeler (#669): müşavir mükellefin Belgeler ekranından
 * dilekçelere geçer, hazır şablonu seçer, yalnız şablonun istediği alanları
 * doldurur; unvan ve VKN kendiliğinden gelir. Önizleme metni gösterir,
 * "Kaydet ve PDF" dilekçeyi kaydeder ve PDF'i açar; kayıtlı dilekçe listede
 * kalır ve PDF'i yeniden alınır.
 *
 * Veri: demo ofisi, Atlas Gıda; her koşuda yeni bir dilekçe (konu benzersiz).
 */
test("dilekçe: Belgeler'den şablon seçilir, önizlenir, kaydedilip PDF alınır (#669)", async ({
  page,
  context,
}) => {
  await page.setViewportSize({ width: 1440, height: 900 });
  const subject = `E2E dilekçe ${Date.now().toString().slice(-6)}`;
  const api = await Api.login(DEMO.owner);
  const engagement = await engagementOf(api, DEMO.atlas.title);

  await uiLogin(page, DEMO.owner);
  await page.goto(officePath(`/engagements/${engagement}/documents`));
  await page.getByRole("link", { name: "Dilekçeler" }).first().click();
  await expect(page).toHaveURL(
    new RegExp(`/engagements/${engagement}/petitions`),
  );
  // Mükellef sayfalarında h1 mükellefin adı; sekme başlığı h2 (#482).
  await expect(
    page.getByRole("heading", { level: 2, name: "Dilekçeler" }),
  ).toBeVisible();

  await page.getByRole("combobox", { name: "Şablon" }).click();
  await page.getByRole("option", { name: "Genel dilekçe" }).click();
  await page
    .getByLabel("Muhatap (kurum)")
    .fill("KADIKÖY VERGİ DAİRESİ MÜDÜRLÜĞÜNE");
  // "Konum" gezinme şeridi de "Konu" ile başlar; alan rol ve tam adla
  // bulunur (etiket metni zorunlu alan imiyle "Konu*").
  await page.getByRole("textbox", { name: "Konu", exact: true }).fill(subject);
  await page.getByLabel("Metin").fill("Gereğinin yapılmasını arz ederim.");

  await page.getByRole("button", { name: "Önizle" }).click();
  const preview = page.getByText(subject).first();
  await expect(preview).toBeVisible();
  await expect(page.getByText(DEMO.atlas.title).first()).toBeVisible();

  const popup = context.waitForEvent("page");
  await page.getByRole("button", { name: "Kaydet ve PDF" }).click();
  await (await popup).close();
  await expect(page.getByText("Dilekçe kaydedildi.")).toBeVisible();

  // Kayıtlı dilekçe listede; PDF'i yeniden alınır.
  const list = await api.json<{
    items: {
      uuid: string;
      rendered: { subject: string; petitioner: string };
    }[];
  }>(await api.get(`/v1/tenant/engagements/${engagement}/petitions?limit=20`));
  const saved = list.items.find((p) => p.rendered.subject === subject);
  expect(saved, "kayıtlı dilekçe").toBeTruthy();
  const pdf = await api.get(
    `/v1/tenant/engagements/${engagement}/petitions/${saved!.uuid}/pdf`,
  );
  expect(pdf.status()).toBe(200);
  expect(pdf.headers()["content-type"]).toContain("application/pdf");
  expect((await pdf.body()).subarray(0, 4).toString("latin1")).toBe("%PDF");
  await api.dispose();
});

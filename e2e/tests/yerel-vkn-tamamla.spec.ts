import { expect, test } from "@playwright/test";

import {
  addConsentedTaxpayer,
  Api,
  DEMO,
  deriveUbl,
  officePath,
  uiLogin,
  uniqueVkn,
  waitForUploads,
} from "./lib";

/**
 * Geçersiz/eksik VKN'nin ofiste tamamlanması (INVALID_TAX_ID): kaynakta
 * karşı tarafın numarası geçersizse fatura Ba/Bs'ye ve numarayla banka
 * eşleşmesine girmez. Müşavir belge yan panelinde "VKN'yi gir" ile doğru
 * numarayı yazar; sunucu denetler, cariyi bağlar, uyarıyı çözülmüş yapar,
 * kimin ne zaman girdiğini tutar. Geçersiz numara 400, ikinci giriş 422.
 *
 * Veri: her koşuda yeni bir mükellef; ona demo atlas-01'den türetilen bir
 * satış faturası yüklenir, alıcının TCKN'si 9 haneye indirilir. Okuyucu
 * VKN/TCKN'yi uzunlukla ayırır (10 VKN, 11 TCKN): 11 haneli bir numara
 * geçerli sayılır, uyarı çıkmaz; geçersiz uzunluktaki numara saklanmaz ve
 * INVALID_TAX_ID açılır.
 */
test("geçersiz VKN'li faturada numara panelden girilir, uyarı çözülür", async ({
  page,
  browser,
}) => {
  test.setTimeout(240_000);
  const run = `${Date.now()}`.slice(-6);
  const taxpayer = { vkn: uniqueVkn(), title: `VKN Tamamla ${run} A.Ş.` };
  const counterparty = uniqueVkn();
  const api = await Api.login(DEMO.owner);
  const eng = await addConsentedTaxpayer(browser, api, taxpayer, "vkn");

  const number = `VKN${new Date().getFullYear()}${String(Date.now()).slice(-9)}`;
  const ubl = deriveUbl(
    "atlas-01-temel-satis.xml",
    taxpayer.vkn,
    taxpayer.title,
  );
  const xml = ubl.buffer
    .toString("utf8")
    .replace(/schemeID="TCKN">\d+</, 'schemeID="TCKN">123456789<')
    .replace(/(<ID\b[^>]*>)ATL\d+(<\/ID>)/, `$1${number}$2`);
  expect(xml).toContain('schemeID="TCKN">123456789<');
  await api.json(
    await api.upload(`/v1/tenant/engagements/${eng}/ubl-uploads`, {
      ...ubl,
      name: `e2e-vkn-${number}.xml`,
      buffer: Buffer.from(xml, "utf8"),
    }),
  );
  await waitForUploads(api, eng, 1);

  type Warning = { code: string; status: string };
  type Inv = {
    uuid: string;
    invoice_number: string;
    counterparty_tax_id?: string;
    warnings?: Warning[];
    counterparty_tax_id_entry?: { set_by_name: string; set_at: string } | null;
  };
  let invoice: Inv | undefined;
  await expect
    .poll(
      async () => {
        const list = await api.json<{ items: Inv[] }>(
          await api.get(`/v1/tenant/engagements/${eng}/invoices?limit=100`),
        );
        invoice = list.items.find((i) => i.invoice_number === number);
        return !!invoice;
      },
      { timeout: 90_000 },
    )
    .toBe(true);
  const url = `/v1/tenant/engagements/${eng}/invoices/${invoice!.uuid}`;
  const before = await api.json<Inv>(await api.get(url));
  expect(before.counterparty_tax_id ?? "").toBe("");
  expect(
    before.warnings?.some(
      (w) => w.code === "INVALID_TAX_ID" && w.status === "open",
    ),
  ).toBe(true);
  expect(before.counterparty_tax_id_entry).toBeNull();

  // Sunucu: denetim haneli olmayan numara reddedilir.
  const bad = await api.put(`${url}/counterparty-tax-id`, {
    tax_id: "1234567891", // son hane denetimi tutmaz (1234567890 geçerli VKN)
  });
  expect(bad.status()).toBe(400);
  expect(await bad.text()).toContain("INVALID_TAX_ID");

  // Panel: uyarının yanında numara girilir.
  await uiLogin(page, DEMO.owner);
  await page.goto(
    officePath(`/engagements/${eng}/documents?belge=${invoice!.uuid}`),
  );
  const box = page.getByRole("region", { name: "VKN/TCKN eksik" });
  await expect(box).toBeVisible();
  await box
    .getByRole("textbox", { name: "Karşı tarafın VKN/TCKN'si" })
    .fill(counterparty);
  await box.getByRole("button", { name: "VKN'yi gir" }).click();
  await expect(
    page.getByText("VKN/TCKN kaydedildi; cari bağlandı, uyarı çözüldü."),
  ).toBeVisible();
  await expect(box).toHaveCount(0);
  await expect(
    page.getByText(/Karşı tarafın VKN\/TCKN'sini ofiste .+ girdi/),
  ).toBeVisible();

  // Sunucu: numara ve kim/ne zaman kaydı tutuldu, uyarı çözüldü.
  const after = await api.json<Inv>(await api.get(url));
  expect(after.counterparty_tax_id).toBe(counterparty);
  expect(after.counterparty_tax_id_entry?.set_by_name).toBeTruthy();
  const w = after.warnings?.find((x) => x.code === "INVALID_TAX_ID");
  expect(w?.status).toBe("resolved");

  // İkinci kez girilemez.
  const again = await api.put(`${url}/counterparty-tax-id`, {
    tax_id: uniqueVkn(),
  });
  expect(again.status()).toBe(422);
  expect(await again.text()).toContain("TAX_ID_SET");
  await api.dispose();
});

import { expect, test } from "@playwright/test";

import { addConsentedTaxpayer, Api, DEMO, deriveUbl, officePath, uiLogin, uniqueVkn, waitForUploads } from "./lib";

/**
 * Eksik kur (#571): kaynakta kur gelmeyen dövizli fatura KDV hesabına
 * girmez; müşavir belge yan panelinden faturanın tarihindeki kuru girer.
 *
 * Veri: her koşuda yeni bir mükellef (Atlas'ın yükleme sayımına dokunmaz);
 * ona kur bilgisi çıkarılmış bir USD faturası yüklenir (demo atlas-06'dan
 * türetilir; benzersiz numara ve ETTN). Fatura tarihi TCMB kur tablosunda
 * bülteni olmayan eski bir gün: kur otomatik dolmaz (#780), eksik kalır.
 */
test("dövizli faturada eksik kur panelden tamamlanır (#571)", async ({ page, browser }) => {
  test.setTimeout(240_000);
  const run = `${Date.now()}`.slice(-6);
  const taxpayer = { vkn: uniqueVkn(), title: `Kur Dış Ticaret ${run} A.Ş.` };
  const api = await Api.login(DEMO.owner);
  const eng = await addConsentedTaxpayer(browser, api, taxpayer, "kur");

  const today = new Date().toISOString().slice(0, 10);
  // Demo ve worker kurları son aylar için; bu günün bülteni tabloda yok.
  const docDate = "2025-03-14";
  const number = `EKR${today.slice(0, 4)}${String(Date.now()).slice(-9)}`;
  const ubl = deriveUbl("atlas-06-doviz-usd.xml", taxpayer.vkn, taxpayer.title);
  const xml = ubl.buffer
    .toString("utf8")
    .replace(/<PricingExchangeRate\b[\s\S]*?<\/PricingExchangeRate>\s*/, "")
    .replace(/(<ID\b[^>]*>)ATL\d+(<\/ID>)/, `$1${number}$2`)
    .split("2026-10-01")
    .join(docDate);
  expect(xml).not.toContain("CalculationRate");
  await api.json(
    await api.upload(`/v1/tenant/engagements/${eng}/ubl-uploads`, {
      ...ubl,
      name: `e2e-eksik-kur-${number}.xml`,
      buffer: Buffer.from(xml, "utf8"),
    }),
  );

  await waitForUploads(api, eng, 1);

  type Inv = { uuid: string; invoice_number: string; currency: string; exchange_rate?: string };
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
  expect(invoice!.currency).toBe("USD");
  expect(invoice!.exchange_rate ?? "").toBe("");

  await uiLogin(page, DEMO.owner);
  await page.goto(officePath(`/engagements/${eng}/documents?belge=${invoice!.uuid}`));
  const box = page.getByRole("region", { name: "Kur eksik: USD fatura" });
  await expect(box).toBeVisible();
  await box.getByRole("textbox", { name: "USD kuru (TL)" }).fill("38,5");
  await box.getByRole("button", { name: "Kuru kaydet" }).click();
  await expect(page.getByText("Kur kaydedildi; KDV ve TL tutarı güncellendi.")).toBeVisible();
  await expect(box).toHaveCount(0);

  // Sunucu kuru tuttu; ikinci kez girilemez.
  const detail = await api.json<Inv>(await api.get(`/v1/tenant/engagements/${eng}/invoices/${invoice!.uuid}`));
  expect(Number(detail.exchange_rate)).toBe(38.5);
  const again = await api.put(`/v1/tenant/engagements/${eng}/invoices/${invoice!.uuid}/exchange-rate`, { rate: "40" });
  expect(again.status()).toBe(409);
  expect(await again.text()).toContain("EXCHANGE_RATE_SET");
  await api.dispose();
});

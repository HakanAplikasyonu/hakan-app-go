import { expect, test } from "@playwright/test";

import { Api, DEMO, deriveUbl, engagementOf, officePath, uiLogin } from "./lib";

/**
 * Eksik kur (#571): kaynakta kur gelmeyen dövizli fatura KDV hesabına
 * girmez; müşavir belge yan panelinden faturanın tarihindeki kuru girer.
 *
 * Veri: Atlas Gıda'ya bu ayın tarihli, kur bilgisi çıkarılmış bir USD
 * faturası yüklenir (demo atlas-06'dan türetilir; benzersiz numara ve ETTN).
 * Bu ay kilitli değildir; dosya yalnızca kendi yüklediği faturaya dokunur.
 */
test("dövizli faturada eksik kur panelden tamamlanır (#571)", async ({ page }) => {
  const api = await Api.login(DEMO.owner);
  const atlas = await engagementOf(api, DEMO.atlas.title);

  const today = new Date().toISOString().slice(0, 10);
  const number = `EKR${today.slice(0, 4)}${String(Date.now()).slice(-9)}`;
  const ubl = deriveUbl("atlas-06-doviz-usd.xml", DEMO.atlas.taxId, DEMO.atlas.title);
  const xml = ubl.buffer
    .toString("utf8")
    .replace(/<PricingExchangeRate\b[\s\S]*?<\/PricingExchangeRate>\s*/, "")
    .replace(/(<ID\b[^>]*>)ATL\d+(<\/ID>)/, `$1${number}$2`)
    .split("2026-10-01")
    .join(today);
  expect(xml).not.toContain("CalculationRate");
  await api.json(
    await api.upload(`/v1/tenant/engagements/${atlas}/ubl-uploads`, {
      ...ubl,
      name: `e2e-eksik-kur-${number}.xml`,
      buffer: Buffer.from(xml, "utf8"),
    }),
  );

  type Inv = { uuid: string; invoice_number: string; currency: string; exchange_rate?: string };
  let invoice: Inv | undefined;
  await expect
    .poll(
      async () => {
        const list = await api.json<{ items: Inv[] }>(
          await api.get(`/v1/tenant/engagements/${atlas}/invoices?limit=100`),
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
  await page.goto(officePath(`/engagements/${atlas}/documents?belge=${invoice!.uuid}`));
  const box = page.getByRole("region", { name: "Kur eksik: USD fatura" });
  await expect(box).toBeVisible();
  await box.getByRole("textbox", { name: "USD kuru (TL)" }).fill("38,5");
  await box.getByRole("button", { name: "Kuru kaydet" }).click();
  await expect(page.getByText("Kur kaydedildi; KDV ve TL tutarı güncellendi.")).toBeVisible();
  await expect(box).toHaveCount(0);

  // Sunucu kuru tuttu; ikinci kez girilemez.
  const detail = await api.json<Inv>(await api.get(`/v1/tenant/engagements/${atlas}/invoices/${invoice!.uuid}`));
  expect(Number(detail.exchange_rate)).toBe(38.5);
  const again = await api.put(`/v1/tenant/engagements/${atlas}/invoices/${invoice!.uuid}/exchange-rate`, { rate: "40" });
  expect(again.status()).toBe(409);
  expect(await again.text()).toContain("EXCHANGE_RATE_SET");
  await api.dispose();
});

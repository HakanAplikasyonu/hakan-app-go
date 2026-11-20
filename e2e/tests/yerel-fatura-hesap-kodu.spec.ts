import { expect, test } from "@playwright/test";

import { addConsentedTaxpayer, Api, DEMO, deriveUbl, officePath, uiLogin, uniqueVkn, waitForUploads } from "./lib";

/**
 * Fatura detayında hesap kodu (#669 F): "Hesap kodu öner" kullanıcıyı
 * inceleme kuyruğuna göndermez; öneriler aynı sayfada "Hesap kodları"
 * bölümünde görünür, satırın kodu orada değiştirilir ve kaydedilir. Sunucu:
 * öneri listesi `invoice` süzgeciyle yalnız o faturanın önerilerini döner.
 *
 * Veri: demo ofisine her koşuda yeni bir mükellef; Atlas'ın gelen faturası
 * (atlas-05, Ekim 2026) bu mükellefe türetilir. Başka dosyanın kaydına
 * dokunmaz.
 */
test.describe.configure({ mode: "serial" });

const run = `${Date.now()}`.slice(-6);
const taxpayer = { vkn: uniqueVkn(), title: `Fatura Kod ${run} A.Ş.` };
const ITEM = "Ofis Koltuğu";
const CODE = "255.02";

type Suggestion = {
  uuid: string;
  status: string;
  target_type: string;
  target_field: string;
  applied_value: unknown;
  provenance: { inputs: Record<string, unknown> };
};

test.describe("fatura detayında hesap kodu (#669 F)", () => {
  let engagement = "";
  let invoice = "";
  let other = "";
  let api: Api;

  const base = () => `/v1/tenant/engagements/${engagement}`;
  const forInvoice = async (id: string, status = "all") =>
    (
      await api.json<{ items: Suggestion[]; total: number }>(
        await api.get(`${base()}/suggestions?status=${status}&kind=account_code&invoice=${id}&limit=100`),
      )
    ).items;

  test.beforeAll(async ({ browser }) => {
    test.setTimeout(240_000);
    api = await Api.login(DEMO.owner);
    engagement = await addConsentedTaxpayer(browser, api, taxpayer, "fkod");

    // İki fatura: süzgecin öbür faturanın önerilerini dışarıda bıraktığı görülür.
    for (const f of ["atlas-05-iade-alis.xml", "atlas-01-temel-satis.xml"]) {
      const up = await api.upload(`${base()}/ubl-uploads`, deriveUbl(f, taxpayer.vkn, taxpayer.title));
      expect(up.status(), await up.text()).toBeLessThan(300);
    }
    await waitForUploads(api, engagement, 2);
    const items = (
      await api.json<{ items: { uuid: string; invoice_number: string }[] }>(await api.get(`${base()}/invoices?limit=10`))
    ).items;
    expect(items.length).toBeGreaterThanOrEqual(2);
    // atlas-05'in kalemi "Ofis Koltuğu": hangi fatura olduğunu satırından bul.
    for (const it of items) {
      const d = await api.json<{ items: { name: string }[] }>(await api.get(`${base()}/invoices/${it.uuid}`));
      if (d.items.some((l) => l.name === ITEM)) invoice = it.uuid;
      else other ||= it.uuid;
    }
    expect(invoice).not.toBe("");
    expect(other).not.toBe("");
    // Öbür faturanın önerileri API ile; bu faturanınki ekrandan istenir.
    await api.json(await api.post(`${base()}/invoices/${other}/account-suggestions`, {}));
  });

  test.afterAll(async () => {
    await api?.dispose();
  });

  test("öneri sayfada kalır; satırın kodu yerinde değiştirilir", async ({ page }) => {
    expect((await forInvoice(other)).length).toBeGreaterThan(0);

    await uiLogin(page, DEMO.owner);
    const url = officePath(`/engagements/${engagement}/invoices/${invoice}`);
    await page.goto(url);
    const panel = page.locator("#hesap-kodlari");
    await expect(panel).toContainText("Hesap kodları");

    await page.getByRole("button", { name: "Hesap kodu öner" }).click();
    // Yükleme sırasında otomatik öneri çalıştıysa "önerilecek alan yok" der.
    await expect(page.getByText(/öneri hazır; aşağıda Hesap kodları|Önerilecek alan yok/)).toBeVisible();
    // Kuyruğa gidilmez: adres aynı.
    expect(new URL(page.url()).pathname).toBe(new URL(url, page.url()).pathname);

    const rows = panel.getByTestId("invoice-account-code");
    await expect(rows.first()).toBeVisible();
    // Süzgeç: yalnız bu faturanın önerileri, ekranla API aynı sayıda alan.
    const mine = (await forInvoice(invoice)).filter((s) => !["superseded", "reverted"].includes(s.status));
    expect(mine.length).toBeGreaterThan(0);
    expect(mine.every((s) => s.provenance.inputs.invoice_uuid === invoice)).toBe(true);
    const fields = new Set(mine.map((s) => `${s.target_type}:${s.target_field}:${String(s.provenance.inputs.line_no ?? "")}`));
    await expect(rows).toHaveCount(fields.size);

    // Kalemin satır hesabı değiştirilir.
    const line = rows.filter({ hasText: ITEM }).filter({ hasText: "Satır hesabı" });
    await expect(line).toHaveCount(1);
    await line.getByRole("button", { name: "Değiştir" }).click();
    await line.getByLabel("Satır hesabı kodu").fill(CODE);
    await line.getByRole("button", { name: "Kaydet" }).click();
    await expect(page.getByText(`Hesap kodu ${CODE} olarak kaydedildi.`)).toBeVisible();
    await expect(line).toContainText(CODE);
    await expect(line).toContainText("Düzeltildi");
    expect(new URL(page.url()).pathname).toBe(new URL(url, page.url()).pathname);

    // Veritabanı: o alanın önerisi yeni kodla düzeltildi.
    const corrected = (await forInvoice(invoice, "corrected")).filter(
      (s) => s.target_type === "invoice_item" && s.target_field === "account_code" && s.provenance.inputs.item_name === ITEM,
    );
    expect(corrected).toHaveLength(1);
    expect(JSON.stringify(corrected[0].applied_value)).toContain(CODE);

    // Geçersiz uuid süzgeci 400.
    expect((await api.get(`${base()}/suggestions?invoice=fatura`)).status()).toBe(400);
  });
});

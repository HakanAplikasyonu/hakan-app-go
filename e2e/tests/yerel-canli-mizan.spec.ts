import { expect, test } from "@playwright/test";

import { addConsentedTaxpayer, Api, DEMO, officePath, uiLogin, uniqueVkn } from "./lib";

/**
 * Canlı mizan (#669 E): Mizan sekmesinde "Kendi fişlerimiz" kaynağı.
 * Mizan ofisin kendi fişlerinden, dönemin yılının Ocak'ından seçilen aya
 * birikimli kurulur; varsayılan onaylı fişler, istenirse taslaklar da.
 * Aynı dönemin yüklenen Luca mizanıyla karşılaştırılır; Excel / PDF iner.
 *
 * Veri: her koşuda yeni bir mükellef. Ekim Luca mizanı (hesap adları için)
 * API ile yüklenir; Eylül ve Ekim'de birer onaylı, Ekim'de bir taslak fiş
 * API ile yazılır. Başka dosyanın kaydına dokunmaz.
 */
test.describe.configure({ mode: "serial" });

const run = `${Date.now()}`.slice(-6);
const taxpayer = { vkn: uniqueVkn(), title: `Canlı Mizan ${run} A.Ş.` };

const october = {
  name: `canli-mizan-${run}.csv`,
  mimeType: "text/csv",
  buffer: Buffer.from(
    [
      "MİZAN 01.10.2026 - 31.10.2026",
      "Hesap Kodu;Hesap Adı;Borç;Alacak;Borç Bakiye;Alacak Bakiye",
      "100;KASA;1.500,00;0,00;1.500,00;",
      "100.01;MERKEZ KASA;1.500,00;0,00;1.500,00;",
      "320;SATICILAR;0,00;1.500,00;;1.500,00",
      "320.01;TEDARİKÇİ A.Ş.;0,00;1.500,00;;1.500,00",
    ].join("\n") + "\n",
    "utf8",
  ),
};

const entry = (date: string, amount: string, description: string) => ({
  entry_date: `${date}T09:00:00Z`,
  description,
  lines: [
    { account: "100.01", debit: amount, credit: "0" },
    { account: "320.01", debit: "0", credit: amount },
  ],
});

test.describe("canlı mizan (#669 E)", () => {
  let engagement = "";
  let api: Api;
  const base = () => `/v1/tenant/engagements/${engagement}`;

  test.beforeAll(async ({ browser }) => {
    test.setTimeout(240_000);
    api = await Api.login(DEMO.owner);
    engagement = await addConsentedTaxpayer(browser, api, taxpayer, "canlimizan");
    const up = await api.upload(`${base()}/account-plan/imports`, october);
    expect(up.status(), await up.text()).toBe(201);

    // Eylül 100 + Ekim 1.500 onaylı; Ekim 200 taslak.
    for (const [date, amount, approve] of [
      ["2026-09-10", "100.00", true],
      ["2026-10-15", "1500.00", true],
      ["2026-10-20", "200.00", false],
    ] as const) {
      const res = await api.post(`${base()}/journal-drafts`, entry(date, amount, `Canlı mizan ${run} ${date}`));
      expect(res.status(), await res.text()).toBe(201);
      if (!approve) continue;
      const draft = await api.json<{ uuid: string }>(res);
      const ok = await api.post(`${base()}/journal-drafts/${draft.uuid}/approve`);
      expect(ok.status(), await ok.text()).toBe(200);
    }
  });

  test.afterAll(async () => {
    await api?.dispose();
  });

  test("kendi fişlerimizden: birikimli, taslaklar, Luca ile fark, Excel ve PDF", async ({ page }) => {
    await uiLogin(page, DEMO.owner);
    await page.goto(officePath(`/engagements/${engagement}/trial-balance`));
    await page.getByLabel("Kaynak").selectOption("canli");
    await expect(page).toHaveURL(/kaynak=canli/);

    // Ekim: Ocak – Ekim birikimli, yalnız onaylı fişler (100 + 1.500).
    await page.goto(officePath(`/engagements/${engagement}/trial-balance?kaynak=canli&donem=2026-10`));
    await expect(page.getByText("2026-01 – 2026-10")).toBeVisible();
    const table = page.getByRole("table", { name: "Mizan" });
    const kasa = table.getByRole("row", { name: /100\.01 MERKEZ KASA/ });
    await expect(kasa).toContainText("1.600,00");
    await expect(table.getByRole("row", { name: /^320 SATICILAR/ })).toContainText("1.600,00");

    // Taslaklar da katılınca 1.800.
    await page.getByRole("checkbox", { name: "Taslakları da kat" }).click();
    await expect(kasa).toContainText("1.800,00");
    await page.getByRole("checkbox", { name: "Taslakları da kat" }).click();
    await expect(kasa).toContainText("1.600,00");

    // Luca mizanıyla karşılaştırma: Luca 1.500, fark 100.
    await page.getByRole("checkbox", { name: "Luca mizanıyla karşılaştır" }).click();
    await expect(page.getByText("Luca mizanıyla karşılaştırılan: Ekim 2026")).toBeVisible();
    await expect(table.getByRole("columnheader", { name: "Luca bakiyesi" })).toBeVisible();
    await expect(kasa).toContainText("1.500,00");
    await expect(kasa).toContainText("100,00");

    // Eylül: yalnız Eylül fişi; o dönemin Luca mizanı yok.
    await page.goto(officePath(`/engagements/${engagement}/trial-balance?kaynak=canli&donem=2026-09&luca=1`));
    await expect(page.getByText("Bu dönem için yüklenmiş Luca mizanı yok.")).toBeVisible();
    await expect(table.getByRole("row", { name: /100\.01 MERKEZ KASA/ })).toContainText("100,00");

    // Excel ve PDF.
    const [xlsx] = await Promise.all([page.waitForEvent("download"), page.getByRole("button", { name: "Excel" }).click()]);
    expect(xlsx.suggestedFilename()).toBe("canli_mizan_2026-09.xlsx");
    const [pdf] = await Promise.all([page.waitForEvent("download"), page.getByRole("button", { name: "PDF" }).click()]);
    expect(pdf.suggestedFilename()).toBe("canli_mizan_2026-09.pdf");
  });

  test("canlı mizan ucu: tutarlar, karşılaştırma, dönem ve biçim denetimi", async () => {
    const rep = await api.json<{
      from_period: string;
      totals: { debit: string; credit: string };
      compared_to: { period: string } | null;
      lines: { code: string; balance: string; change: string | null }[];
    }>(await api.get(`${base()}/account-plan/live-trial-balance?period=2026-10&compare=luca`));
    expect(rep.from_period).toBe("2026-01");
    expect(rep.totals).toEqual({ debit: "1600.00", credit: "1600.00" });
    expect(rep.compared_to?.period).toBe("2026-10");
    const kasa = rep.lines.find((l) => l.code === "100.01");
    expect(kasa).toMatchObject({ balance: "1600.00", change: "100.00" });

    const drafts = await api.json<{ totals: { debit: string } }>(
      await api.get(`${base()}/account-plan/live-trial-balance?period=2026-10&include_drafts=true`),
    );
    expect(drafts.totals.debit).toBe("1800.00");

    expect((await api.get(`${base()}/account-plan/live-trial-balance?period=2026-13`)).status()).toBe(400);
    expect((await api.get(`${base()}/account-plan/live-trial-balance?period=2026-10&compare=x`)).status()).toBe(400);
    expect((await api.get(`${base()}/account-plan/live-trial-balance/export?period=2026-10&format=docx`)).status()).toBe(400);
    const res = await api.get(`${base()}/account-plan/live-trial-balance/export?period=2026-10`);
    expect(res.status()).toBe(200);
    expect(res.headers()["content-disposition"]).toContain('filename="canli_mizan_2026-10.xlsx"');
    expect((await res.body()).subarray(0, 2).toString()).toBe("PK");
  });
});

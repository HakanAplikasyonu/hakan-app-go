import { readFileSync } from "node:fs";

import { expect, test } from "@playwright/test";

import { addConsentedTaxpayer, Api, DEMO, officePath, uiLogin, uniqueVkn } from "./lib";

/**
 * Uyarı İncele panelinden gerekçeyle kapatılır (#570): gerekçe yazılmadan
 * kapatılamaz; kapatılan uyarı açık listeden düşer, "Çözüldü" sekmesinde
 * kimin, ne zaman ve hangi gerekçeyle kapattığı görünür.
 *
 * Veri: demo ofisine her koşuda yeni bir mükellef. Ziraat ekstresi
 * (fixtures/atlas-ziraat-2026-10.sta) yüklenir; faturası olmayan 5.400,00
 * tahsilat "Faturası olmayan tahsilat" uyarısını açar. Başka dosyanın
 * kaydına dokunmaz.
 */
const run = `${Date.now()}`.slice(-6);
const taxpayer = { vkn: uniqueVkn(), title: `Uyarı Kapatma ${run} A.Ş.` };
const PERIOD = "2026-10";
const CODE = "BNK_UNINVOICED_COLLECTION";
const reason = `Avans tahsilati; fatura Kasim'da kesilecek (${run})`;

type Warning = { uuid: string; status: string; resolution_note: string | null; resolved_by_name: string | null };

test("uyarı gerekçeyle kapatılır; açık listeden düşer, kim ve neden kapattığı görünür (#570)", async ({ browser, page }) => {
  test.setTimeout(240_000);
  const api = await Api.login(DEMO.owner);
  const engagement = await addConsentedTaxpayer(browser, api, taxpayer, "uyari");
  const base = `/v1/tenant/engagements/${engagement}`;
  const warnings = async (status: string) =>
    api.json<{ items: Warning[]; total: number }>(
      await api.get(`${base}/warnings?status=${status}&code=${CODE}&period=${PERIOD}&limit=5`),
    );

  await api.json(await api.post(`${base}/cash-boxes`, { iban: "TR76 0001 0005 1978 6457 8413 26", name: "Ziraat TL" }));
  await api.json(
    await api.upload(`${base}/bank-statements`, {
      name: "atlas-ziraat-2026-10.sta",
      mimeType: "text/plain",
      buffer: readFileSync("fixtures/atlas-ziraat-2026-10.sta"),
    }),
  );
  const match = await api.json<{ findings: string[] }>(await api.post(`${base}/bank-matching?period=${PERIOD}`));
  expect(match.findings).toContain(CODE);
  const open = await warnings("open");
  expect(open.total).toBe(1);
  const warning = open.items[0];

  // İncele paneli: "Çözüldü" gerekçe ister; yazılmadan kapatılamaz.
  await uiLogin(page, DEMO.owner);
  await page.goto(officePath(`/engagements/${engagement}/warnings?view=list`));
  await page.getByRole("row").filter({ hasText: "Faturası olmayan tahsilat" }).getByText("Faturası olmayan tahsilat").first().click();
  const sheet = page.getByTestId("warning-sheet");
  await expect(sheet).toBeVisible();
  await sheet.getByRole("button", { name: "Çözüldü", exact: true }).first().click();
  const note = sheet.getByLabel("Ne düzeltildi? (gerekçe zorunlu)");
  const confirm = sheet.getByRole("button", { name: "Çözüldü olarak kapat" });
  await expect(confirm).toBeDisabled();
  expect((await warnings("open")).total).toBe(1);
  await note.fill(reason);
  await expect(confirm).toBeEnabled();
  await confirm.click();
  await expect(sheet.getByText("Bu uyarı kapandı.")).toBeVisible();

  // Veritabanı: uyarı kapalı, gerekçe ve kapatan kayıtlı; açık uyarı yok.
  expect((await warnings("open")).total).toBe(0);
  const resolved = (await warnings("resolved")).items.find((w) => w.uuid === warning.uuid);
  expect(resolved).toBeTruthy();
  expect(resolved!.status).toBe("resolved");
  expect(resolved!.resolution_note).toBe(reason);
  expect(resolved!.resolved_by_name).toBeTruthy();

  // Ekran: açık listede yok; Çözüldü sekmesinde gerekçesiyle görünür.
  await page.goto(officePath(`/engagements/${engagement}/warnings?view=list`));
  await expect(page.getByRole("row").filter({ hasText: "Faturası olmayan tahsilat" })).toHaveCount(0);
  await page.goto(officePath(`/engagements/${engagement}/warnings?view=list&status=resolved`));
  const row = page.getByRole("row").filter({ hasText: "Faturası olmayan tahsilat" });
  await expect(row).toHaveCount(1);
  await expect(row.getByTestId("warning-decision")).toContainText(reason);
  await expect(row.getByTestId("warning-decision")).toContainText("Çözüldü");
  await api.dispose();
});

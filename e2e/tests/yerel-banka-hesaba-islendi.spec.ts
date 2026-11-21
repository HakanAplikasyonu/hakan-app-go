import { expect, test } from "@playwright/test";

import { addConsentedTaxpayer, Api, DEMO, officePath, uiLogin, uniqueVkn } from "./lib";

/**
 * Banka listesi biten işi bekleyen gibi göstermez (#710): faturası olmayan
 * ama karşı hesabı onaylanmış hareket (vergi, SGK, maaş, masraf) "Eşleşme"
 * sütununda "Eşleşmedi" yerine "Hesaba işlendi" görünür; özetteki uyarı
 * kartı "Faturası ya da hesabı bekleyen" yalnız gerçekten bekleyenleri sayar
 * (API: summary.unmatched − summary.unmatched_booked).
 *
 * Veri: demo ofisine her koşuda yeni bir mükellef; iki çıkışlı Kasım MT940
 * ekstresi API ile yüklenir, vergi ödemesi API ile 360'a işlenir.
 */
const run = `${Date.now()}`.slice(-6);
const taxpayer = { vkn: uniqueVkn(), title: `Banka İşlendi ${run} A.Ş.` };
const IBAN = "TR760001000519786457841326";

const statement = [
  ":20:E2E710ISL",
  `:25:${IBAN}`,
  ":60F:C261101TRY50000,00",
  ":61:2611051105D7250,00NTRFNONREF//V1",
  ":86:GIB VERGI ODEMESI KDV 2026/10",
  ":61:2611061106D2380,00NTRFNONREF//V2",
  ":86:EFT GIDEN E2E BILINMEYEN ALICI",
  ":62F:C261106TRY40370,00",
  "",
].join("\r\n");

type Summary = { unmatched: number; unmatched_booked: number };
type Row = { uuid: string; amount: string; match_status: string; booked_account_code?: string };

test("banka listesi: hesaba işlenen hareket \"Hesaba işlendi\"; bekleyen sayısı yalnız bekleyenler (#710)", async ({ browser, page }) => {
  test.setTimeout(180_000);
  const api = await Api.login(DEMO.owner);
  const engagement = await addConsentedTaxpayer(browser, api, taxpayer, "islendi");
  const base = `/v1/tenant/engagements/${engagement}`;
  await api.json(await api.post(`${base}/cash-boxes`, { iban: "TR76 0001 0005 1978 6457 8413 26", name: "Ziraat TL" }));
  const up = await api.upload(`${base}/bank-statements`, {
    name: "islendi-2026-11.sta",
    mimeType: "text/plain",
    buffer: Buffer.from(statement, "utf8"),
  });
  expect(up.status(), await up.text()).toBeLessThan(300);
  const list = async () =>
    api.json<{ items: Row[]; summary: Summary }>(await api.get(`${base}/bank-transactions?period=2026-11&limit=50`));
  const tax = (await list()).items.find((x) => x.amount === "7250.00")!;
  const other = (await list()).items.find((x) => x.amount === "2380.00")!;
  expect(tax && other).toBeTruthy();
  const booked = await api.put(`${base}/bank-postings/${tax.uuid}`, { account_code: "360.01", account_name: "KDV" });
  expect(booked.status(), await booked.text()).toBe(204);

  const after = await list();
  const taxRow = after.items.find((x) => x.uuid === tax.uuid)!;
  expect(taxRow.match_status).toBe("unmatched");
  expect(taxRow.booked_account_code).toBe("360.01");
  expect(after.summary.unmatched_booked).toBeGreaterThanOrEqual(1);
  const waiting = after.summary.unmatched - after.summary.unmatched_booked;

  await uiLogin(page, DEMO.owner);
  await page.goto(officePath(`/engagements/${engagement}/bank?period=2026-11`));
  const taxLine = page.getByRole("row").filter({ hasText: "GIB VERGI ODEMESI KDV 2026/10" });
  await expect(taxLine).toContainText("Hesaba işlendi");
  await expect(taxLine).not.toContainText("Eşleşmedi");
  const card = page.getByText("Faturası ya da hesabı bekleyen", { exact: true }).locator("..");
  await expect(card).toHaveText(`${waiting}Faturası ya da hesabı bekleyen`);
  // İşlenmemiş hareket hâlâ bekliyor görünür.
  const otherLine = page.getByRole("row").filter({ hasText: "EFT GIDEN E2E BILINMEYEN ALICI" });
  const otherRow = after.items.find((x) => x.uuid === other.uuid)!;
  await expect(otherLine).toContainText(otherRow.booked_account_code ? "Hesaba işlendi" : "Eşleşmedi");
  await api.dispose();
});

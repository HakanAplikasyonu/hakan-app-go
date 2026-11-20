import { expect, test } from "@playwright/test";

import { addConsentedTaxpayer, Api, DEMO, officePath, uiLogin, uniqueVkn } from "./lib";

/**
 * İnceleme kuyruğu sayıları ve sayfalama (#669 B). Kuyruk 100'den fazla
 * bekleyen öneriyi sayfalarla gösterir; başlıktaki sayı, durum dökümü ve
 * API aynı tanımla sayar; karar verilince döküm güncellenir. Süzgeç ofisin
 * tamamından darken ofis toplamı ayrıca yazılır.
 *
 * Veri: demo ofisine her koşuda yeni bir mükellef. 104 ücret hareketli bir
 * MT940 ekstresi API ile yüklenir; her hareketin karşı hesap önerisi
 * (780, eşiğin altında) kuyruğa düşer. Başka dosyanın kaydına dokunmaz.
 */
const run = `${Date.now()}`.slice(-6);
const taxpayer = { vkn: uniqueVkn(), title: `Kuyruk Sayfa ${run} A.Ş.` };
const PERIOD = "2026-10";
const MOVEMENTS = 104;

/** Açılış 100.000,00; her gün artan tutarda bir ücret çıkışı. */
function statement(): Buffer {
  const lines = [":20:K2KUYRUK", ":25:TR760001000519786457841326", ":60F:C261001TRY100000,00"];
  let spent = 0;
  for (let i = 1; i <= MOVEMENTS; i++) {
    const day = String(1 + (i % 28)).padStart(2, "0");
    const kurus = 1000 + i; // 10,01 … 11,04
    spent += kurus;
    lines.push(`:61:2610${day}10${day}D${Math.floor(kurus / 100)},${String(kurus % 100).padStart(2, "0")}NCOMNONREF//K${i}`);
    lines.push(`:86:EFT UCRETI ${run}-${i}`);
  }
  const left = 10_000_000 - spent;
  lines.push(`:62F:C261028TRY${Math.floor(left / 100)},${String(left % 100).padStart(2, "0")}`);
  return Buffer.from(lines.join("\n") + "\n", "utf8");
}

test("kuyruk 100'den fazla öneriyi sayfalar; başlık, döküm ve API aynı sayıyı verir (#669)", async ({ browser, page }) => {
  test.setTimeout(300_000);
  const api = await Api.login(DEMO.owner);
  const engagement = await addConsentedTaxpayer(browser, api, taxpayer, "kuyruk");
  const base = `/v1/tenant/engagements/${engagement}`;
  const total = async (status: string) =>
    (await api.json<{ total: number }>(await api.get(`${base}/suggestions?status=${status}&limit=1`))).total;

  await api.json(await api.post(`${base}/cash-boxes`, { iban: "TR76 0001 0005 1978 6457 8413 26", name: "Ziraat TL" }));
  await api.json(
    await api.upload(`${base}/bank-statements`, { name: `kuyruk-${run}.sta`, mimeType: "text/plain", buffer: statement() }),
  );
  await api.json(await api.post(`${base}/bank-postings/suggest`, { period: PERIOD }));
  const pending = await total("pending");
  expect(pending, "100'den fazla bekleyen öneri").toBeGreaterThan(100);

  await uiLogin(page, DEMO.owner);
  await page.goto(officePath(`/engagements/${engagement}/review`));
  await expect(page.getByRole("heading", { level: 2, name: "İnceleme kuyruğu" })).toBeVisible();

  // Başlık, döküm ve API aynı sayı; ofis toplamı ayrıca yazılır.
  await expect(page.getByText(`${pending} öneri bekliyor`)).toBeVisible();
  const counts = page.getByTestId("review-counts");
  await expect(counts).toContainText(`Bekleyen ${pending} ·`);
  await expect(counts).toContainText("Onaylanan 0 ·");
  const office = Number((await page.getByTestId("review-office-total").innerText()).match(/bekleyen (\d+)/)?.[1]);
  expect(office).toBeGreaterThanOrEqual(pending);

  // Sayfalama: ilk 100, sonra kalanlar; geri dönülür.
  const pager = page.getByTestId("review-pager");
  const items = page.getByRole("listitem").filter({ hasText: "EFT UCRETI" });
  await expect(pager).toContainText(`1–100 / ${pending}`);
  await expect(items).toHaveCount(100);
  await expect(pager.getByRole("button", { name: "Önceki 100" })).toBeDisabled();
  await pager.getByRole("button", { name: "Sonraki 100" }).click();
  await expect(pager).toContainText(`101–${pending} / ${pending}`);
  await expect(items).toHaveCount(pending - 100);
  await expect(pager.getByRole("button", { name: "Sonraki 100" })).toBeDisabled();
  await pager.getByRole("button", { name: "Önceki 100" }).click();
  await expect(pager).toContainText(`1–100 / ${pending}`);
  await expect(items).toHaveCount(100);

  // Bir öneri onaylanır: bekleyen bir azalır, onaylanan bir artar (ekran = API).
  await items.first().click();
  await page.getByRole("article").getByRole("button", { name: "Onayla", exact: true }).first().click();
  await expect(counts).toContainText(`Bekleyen ${pending - 1} ·`);
  await expect(counts).toContainText("Onaylanan 1 ·");
  await expect(page.getByText(`${pending - 1} öneri bekliyor`)).toBeVisible();
  expect(await total("pending")).toBe(pending - 1);
  expect(await total("accepted")).toBe(1);
  await api.dispose();
});

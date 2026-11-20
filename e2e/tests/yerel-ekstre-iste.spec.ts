import { readFileSync } from "node:fs";

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
 * "Mükelleften iste" (#710, Oğuz demo: Ilgaz Kuruyemiş, Yapı Kredi ekstresi
 * eksik): ofis beyan tablosunda eksik ekstre için istek açılınca ne
 * istendiği, ne zaman istendiği ve durumu görünür; sayfa yenilense de
 * kaybolmaz, aynı ekstre ikinci kez istenmez ve isteğe gidilir.
 *
 * Veri: her koşuda yeni mükellef; Ziraat hesabına Eylül 2026 ekstresi
 * yüklenir, Ekim 2026 ekstresi yok (eksik ekstre kuralı, #568).
 */
const run = `${Date.now()}`.slice(-6);
const taxpayer = { vkn: uniqueVkn(), title: `Ekstre İste ${run} Ltd. Şti.` };
const PERIOD = "2026-10";
const title = "Ekim 2026 Ziraat TL banka ekstresi";

test("eksik ekstre: istek tarihi ve durumu görünür, ikinci kez istenmez (#710)", async ({
  browser,
  page,
}) => {
  test.setTimeout(240_000);
  const api = await Api.login(DEMO.owner);
  const engagement = await addConsentedTaxpayer(
    browser,
    api,
    taxpayer,
    "ekstreiste",
  );
  const base = `/v1/tenant/engagements/${engagement}`;
  await api.json(
    await api.post(`${base}/cash-boxes`, {
      iban: "TR76 0001 0005 1978 6457 8413 26",
      name: "Ziraat TL",
    }),
  );
  // Ekim örneği Eylül'e kaydırılır: Ekim'de hareket yok, önceki ayda var.
  const september = readFileSync("fixtures/atlas-ziraat-2026-10.sta", "utf8")
    .replace(/(:60F:C|:62F:C|:61:)2610/g, "$12609")
    .replace(/(:61:\d{6})10/g, "$109");
  const up = await api.upload(`${base}/bank-statements`, {
    name: "ekstre-iste-2026-09.sta",
    mimeType: "text/plain",
    buffer: Buffer.from(september),
  });
  expect(up.status(), await up.text()).toBeLessThan(300);

  await uiLogin(page, DEMO.owner);
  await page.goto(officePath(`/tax-returns?period=${PERIOD}`));
  await page.getByLabel("Göster").selectOption("documents");
  const row = page.getByRole("row", { name: new RegExp(taxpayer.title) });
  await expect(row).toContainText("Banka ekstresi eksik: Ziraat TL");

  await row.getByRole("button", { name: "Mükelleften iste" }).click();
  await expect(
    page.getByText("Ekstre mükelleften istendi; görev listesine düştü."),
  ).toBeVisible();
  const today = new Date().toLocaleDateString("tr-TR", {
    timeZone: "Europe/Istanbul",
  });
  const state = row.getByTestId("statement-request-state");
  await expect(state).toContainText(`${today} tarihinde istendi`);
  await expect(state).toContainText("mükellef bekleniyor, son gün");
  await expect(
    row.getByRole("button", { name: "Mükelleften iste" }),
  ).toHaveCount(0);

  // Sayfa yenilense de durum kalır; istek bir tanedir.
  await page.reload();
  await page.getByLabel("Göster").selectOption("documents");
  await expect(state).toContainText(`${today} tarihinde istendi`);
  await expect(
    row.getByRole("button", { name: "Mükelleften iste" }),
  ).toHaveCount(0);
  const tasks = await api.json<{
    items: { kind: string; title: string; status: string }[];
  }>(await api.get(`${base}/document-tasks?status=all&period=${PERIOD}`));
  expect(
    tasks.items.filter(
      (x) => x.kind === "document_request" && x.title === title,
    ),
  ).toEqual([expect.objectContaining({ status: "open", title })]);

  // İsteğe git: mükellef ekranında açık istek görünür.
  await state.getByRole("link", { name: "İsteği gör" }).click();
  await expect(page).toHaveURL(new RegExp(`/engagements/${engagement}$`));
  await expect(page.getByText(title).first()).toBeVisible();
  await api.dispose();
});

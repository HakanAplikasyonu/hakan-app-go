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
 * Cari kartları Excel ile toplu içe aktarılır (#669): Excel içe aktarma
 * ekranında veri türü "Cari kartları" seçilir, müşteri / tedarikçi listesi
 * yüklenir; kolonlar başlıklardan eşlenir, "Kayda işle" kartları yazar.
 * VKN'si hatalı satır yazılmaz, nedeni satır numarasıyla döner. Yazılan
 * kartlar hesap planı alt hesap üretiminde (120 / 320) görünür.
 *
 * Veri: demo ofisine her koşuda yeni bir mükellef; kartlar onundur.
 */
test("cari listesi Excel'den içe aktarılır, alt hesap üretiminde görünür (#669)", async ({
  page,
  browser,
}) => {
  test.setTimeout(120_000);
  await page.setViewportSize({ width: 1440, height: 900 });
  const run = `${Date.now()}`.slice(-6);
  const api = await Api.login(DEMO.owner);
  const engagement = await addConsentedTaxpayer(
    browser,
    api,
    { vkn: uniqueVkn(), title: `Cari Excel ${run} A.Ş.` },
    "cari",
  );
  const base = `/v1/tenant/engagements/${engagement}`;
  const buyer = `Alıcı ${run} Ltd. Şti.`;
  const seller = `Satıcı ${run} A.Ş.`;
  const csv = [
    "Ünvan;VKN/TCKN;Cari Türü;Vergi Dairesi",
    `${buyer};${uniqueVkn()};Müşteri;Kadıköy`,
    `${seller};${uniqueVkn()};Tedarikçi;Beşiktaş`,
    `Hatalı ${run};12345;Müşteri;Kadıköy`,
  ].join("\n");
  const fileName = `cari-${run}.csv`;

  await uiLogin(page, DEMO.owner);
  await page.goto(officePath(`/engagements/${engagement}/excel-imports`));
  await page.getByRole("combobox", { name: "Veri türü" }).first().click();
  await page.getByRole("option", { name: "Cari kartları" }).click();
  await page.locator('input[type="file"]').setInputFiles({
    name: fileName,
    mimeType: "text/csv",
    buffer: Buffer.from(csv, "utf8"),
  });
  await expect(page.getByText("İşlemeye hazır").first()).toBeVisible();
  await page.getByRole("button", { name: "Kayda işle" }).first().click();
  const dialog = page.getByRole("alertdialog").or(page.getByRole("dialog"));
  await dialog.getByRole("button", { name: "Kayda işle" }).click();
  await expect(page.getByText("İşlendi").first()).toBeVisible();

  type Import = {
    file_name: string;
    status: string;
    data_type: string | null;
    summary: {
      added: number;
      duplicates: number;
      rejected?: number;
      row_errors?: { row_no: number; reason: string }[];
    } | null;
  };
  const imports = await api.json<{ items: Import[] }>(
    await api.get(`${base}/excel-imports?limit=20`),
  );
  const done = imports.items.find((x) => x.file_name === fileName);
  expect(done, "yüklenen dosya").toBeTruthy();
  expect(done!.data_type).toBe("customers");
  expect(done!.status).toBe("committed");
  expect(done!.summary?.added).toBe(2);
  expect(done!.summary?.rejected).toBe(1);
  expect(done!.summary?.row_errors?.[0]?.reason).toContain("VKN 10");

  // Kartlar alt hesap üretimine girer: alıcı 120, satıcı 320.
  const gen = await api.json<{
    created: { main_code: string; name: string }[];
  }>(
    await api.post(`${base}/account-plan/subaccounts/generate`, {
      main_codes: ["120", "320"],
    }),
  );
  const mains = (name: string) =>
    gen.created.filter((s) => s.name === name).map((s) => s.main_code);
  expect(mains(buyer)).toEqual(["120"]);
  expect(mains(seller)).toEqual(["320"]);
  await api.dispose();
});

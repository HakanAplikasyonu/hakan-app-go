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
 * Mükellef defterleri (#780): mükellef ekranının "Defterler" sekmesi. Ofis
 * Luca'dan aldığı işletme defteri dökümünü ve e-defter yevmiyesini döneme
 * yükler; dönem listesi, eksik dönem uyarısı, Defter dönemi (aylık / 3
 * aylık) ve dosyadan okunan özet. Okunamayan dosya (bozuk .xls) saklanır.
 *
 * Veri: demo ofisine her koşuda yeni bir mükellef; dosyalar ona yüklenir.
 */
test.describe.configure({ mode: "serial" });

const run = `${Date.now()}`.slice(-6);
const taxpayer = { vkn: uniqueVkn(), title: `Defter ${run} Ltd.` };

const csv = (name: string, period: string, buy: string) => ({
  name,
  mimeType: "text/csv",
  buffer: Buffer.from(
    [
      `İŞLETME DEFTERİ ${period} ${run}`,
      "Tarih;Evrak No;Açıklama;Satın Alınan Mallar;Giderler;KDV;Satılan Mallar",
      `03.${period.slice(5)}.${period.slice(0, 4)};A1;Toptancı mal alışı;${buy};;200,00;`,
      `10.${period.slice(5)}.${period.slice(0, 4)};A2;Dükkân kirası;;750,00;;`,
      `20.${period.slice(5)}.${period.slice(0, 4)};S1;Perakende satış;;;;4.250,50`,
    ].join("\n"),
    "utf8",
  ),
});

const yevmiye = (period: string) => ({
  name: `${taxpayer.vkn}-${period.replace("-", "")}-Y-000001.xml`,
  mimeType: "application/xml",
  buffer: Buffer.from(
    `<?xml version="1.0" encoding="UTF-8"?><edefter:defter xmlns:edefter="http://www.edefter.gov.tr" xmlns:gl-cor="http://www.xbrl.org/int/gl/cor/2006-10-25">` +
      `<gl-cor:accountingEntries><gl-cor:entryHeader>` +
      `<gl-cor:entryDetail><gl-cor:account><gl-cor:accountMainID>153</gl-cor:accountMainID><gl-cor:accountMainDescription>TİCARİ MALLAR</gl-cor:accountMainDescription></gl-cor:account><gl-cor:amount>1000.00</gl-cor:amount><gl-cor:debitCreditCode>D</gl-cor:debitCreditCode></gl-cor:entryDetail>` +
      `<gl-cor:entryDetail><gl-cor:account><gl-cor:accountMainID>320</gl-cor:accountMainID><gl-cor:accountMainDescription>SATICILAR</gl-cor:accountMainDescription></gl-cor:account><gl-cor:amount>1000.00</gl-cor:amount><gl-cor:debitCreditCode>C</gl-cor:debitCreditCode></gl-cor:entryDetail>` +
      `</gl-cor:entryHeader></gl-cor:accountingEntries></edefter:defter>`,
    "utf8",
  ),
});

/** "2026-10" → n ay önce. */
const monthsBack = (period: string, n: number) => {
  const [y, m] = period.split("-").map(Number);
  const d = new Date(Date.UTC(y, m - 1 - n, 1));
  return d.toISOString().slice(0, 7);
};

type LedgerList = {
  period_kind: string;
  last_completed: string;
  items: {
    uuid: string;
    kind: string;
    period: string;
    file_name: string;
    readable: boolean;
  }[];
  periods: { period: string; missing: boolean; files: number }[];
  missing: string[];
};

test.describe("mükellef defterleri (#780)", () => {
  let engagement = "";
  let api: Api;
  let last = "";
  const base = () => `/v1/tenant/engagements/${engagement}/ledgers`;
  const list = async () => api.json<LedgerList>(await api.get(base()));

  test.beforeAll(async ({ browser }) => {
    test.setTimeout(240_000);
    api = await Api.login(DEMO.owner);
    engagement = await addConsentedTaxpayer(browser, api, taxpayer, "defter");
    last = (await list()).last_completed;
  });

  test.afterAll(async () => {
    await api?.dispose();
  });

  test("uçlar: yükle, özet, aynı dosya, okunamayan dosya, eksik dönem", async () => {
    // Yeni mükellef: henüz dosya ve beklenen dönem yok.
    const empty = await list();
    expect(empty.period_kind).toBe("monthly");
    expect(empty.items).toEqual([]);

    // İşletme defteri: alış / gider / satış toplamları okunur.
    const file = csv(`isletme-${run}.csv`, last, "1.000,00");
    const res = await api.uploadWith(base(), file, { period: last });
    expect(res.status(), await res.text()).toBe(201);
    const up = (await res.json()).data;
    expect(up.replayed).toBe(false);
    expect(up.item).toMatchObject({
      kind: "isletme_defteri",
      period: last,
      readable: true,
    });
    expect(up.item.summary.isletme).toMatchObject({
      purchases: "1000.00",
      expenses: "750.00",
      sales: "4250.50",
    });

    // Aynı dosya ikinci kez: ilk kayıt döner.
    const again = await api.uploadWith(base(), file, { period: last });
    expect(again.status()).toBe(200);
    expect((await again.json()).data.replayed).toBe(true);

    // e-Defter yevmiyesi: hesap planı ve ana hesap toplamları.
    const ed = await api.uploadWith(base(), yevmiye(last), { period: last });
    expect(ed.status(), await ed.text()).toBe(201);
    const edItem = (await ed.json()).data.item;
    expect(edItem.kind).toBe("e_defter_yevmiye");
    expect(
      edItem.summary.e_defter.accounts.map((a: { code: string }) => a.code),
    ).toEqual(["153", "320"]);
    expect(edItem.summary.e_defter.debit_total).toBe("1000.00");

    // Bozuk .xls: saklanır, okunamadı.
    const xls = {
      name: `eski-${run}.xls`,
      mimeType: "application/vnd.ms-excel",
      buffer: Buffer.concat([
        Buffer.from([0xd0, 0xcf, 0x11, 0xe0]),
        Buffer.from(`eski ${run}`),
      ]),
    };
    const old = await api.uploadWith(base(), xls, {
      period: monthsBack(last, 3),
    });
    expect(old.status(), await old.text()).toBe(201);
    const oldItem = (await old.json()).data.item;
    expect(oldItem.readable).toBe(false);
    expect(oldItem.summary.reason).toBe("legacy_xls");

    // Geçersiz: gelecek dönem, desteklenmeyen dosya, bilinmeyen tür.
    const future = await api.uploadWith(
      base(),
      csv(`gelecek-${run}.csv`, last, "5,00"),
      { period: "2099-01" },
    );
    expect(future.status()).toBe(400);
    const png = {
      name: "resim.png",
      mimeType: "image/png",
      buffer: Buffer.from([
        0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a, 0, 0, 0,
      ]),
    };
    expect((await api.uploadWith(base(), png, { period: last })).status()).toBe(
      400,
    );
    expect(
      (
        await api.uploadWith(base(), csv(`tur-${run}.csv`, last, "6,00"), {
          period: last,
          kind: "yok",
        })
      ).status(),
    ).toBe(400);

    // Üç ay önceki defter yüklendi: aradaki iki ay eksik.
    const after = await list();
    expect(after.items).toHaveLength(3);
    expect(after.missing).toEqual([monthsBack(last, 1), monthsBack(last, 2)]);

    // 3 aylık: dönem çeyreğin son ayı; eksik sayısı değişir.
    expect(
      (
        await api.put(`${base()}/settings`, { period_kind: "quarterly" })
      ).status(),
    ).toBe(200);
    expect((await list()).period_kind).toBe("quarterly");
    expect(
      (await api.put(`${base()}/settings`, { period_kind: "yillik" })).status(),
    ).toBe(400);
    expect(
      (
        await api.put(`${base()}/settings`, { period_kind: "monthly" })
      ).status(),
    ).toBe(200);
  });

  test("ekran: sekme, yardım, eksik uyarısı, özet ve sil", async ({ page }) => {
    await uiLogin(page, DEMO.owner);
    await page.goto(officePath(`/engagements/${engagement}`));
    await page.getByRole("link", { name: "Defterler", exact: true }).click();
    await expect(page).toHaveURL(/\/ledgers/);

    await expect(page.getByText("Nasıl yüklenir?")).toBeVisible();
    await expect(page.getByTestId("ledgers-missing")).toContainText(
      "2 dönemin defteri eksik",
    );

    const files = page.getByRole("table", { name: "Yüklenen dosyalar" });
    const row = files.getByRole("row", { name: new RegExp(`isletme-${run}`) });
    await expect(row).toContainText("İşletme defteri");
    await expect(row).toContainText("Okundu");
    await expect(
      files.getByRole("row", { name: new RegExp(`eski-${run}`) }),
    ).toContainText("Okunamadı");

    // Özet: toplamlar ve en büyük satır.
    await row.getByRole("button", { name: "Özet" }).click();
    const dialog = page.getByRole("dialog", { name: "Defter özeti" });
    await expect(dialog).toContainText("Alışlar");
    await expect(dialog).toContainText("1.000,00");
    await expect(dialog).toContainText("4.250,50");
    await expect(dialog).toContainText("Perakende satış");
    await page.keyboard.press("Escape");

    // Ekrandan yükleme (dönem kutusunda son biten ay seçili).
    await page
      .getByLabel("Dosya (Excel, CSV, XML ya da ZIP)")
      .setInputFiles(csv(`ekran-${run}.csv`, last, "2.000,00"));
    await page.getByRole("button", { name: "Yükle", exact: true }).click();
    await expect(
      files.getByRole("row", { name: new RegExp(`ekran-${run}`) }),
    ).toBeVisible();

    // Defter dönemi: 3 aylık seçilir, kaydedilir.
    await page.getByLabel("Defter dönemi").selectOption("quarterly");
    await expect.poll(async () => (await list()).period_kind).toBe("quarterly");
    await page.getByLabel("Defter dönemi").selectOption("monthly");
    await expect.poll(async () => (await list()).period_kind).toBe("monthly");

    // Sil: onay sorulur, kayıt gider.
    const old = files.getByRole("row", { name: new RegExp(`eski-${run}`) });
    await old.getByRole("button", { name: "Sil" }).click();
    await page
      .getByRole("alertdialog")
      .getByRole("button", { name: "Sil", exact: true })
      .click();
    await expect(old).toHaveCount(0);
    expect(
      (await list()).items.some((i) => i.file_name.startsWith(`eski-${run}`)),
    ).toBe(false);
  });
});

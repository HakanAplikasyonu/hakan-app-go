import { expect, test } from "@playwright/test";

import {
  addConsentedTaxpayer,
  Api,
  DEMO,
  deriveUbl,
  officePath,
  uiLogin,
  uniqueVkn,
  waitForUploads,
} from "./lib";

/**
 * Garanti BBVA ve VakıfBank ekstresi "Yükle" ile (Oğuz'un geri bildirimi:
 * ekran çöktü, hareketler cariyle eşleşmedi).
 *
 * - Garanti BBVA bireysel çıktısı: açıklama "FAST Gelen - AD - not"
 *   biçiminde (etiket yok), tutarda "TL". Karşı taraf adı açıklamadan
 *   okunur; içe aktarma faturayla eşleştirmeyi kendiliğinden çalıştırır,
 *   tahsilat Atlas faturasına "Önerildi" olur (elle "eşleştir" gerekmeden).
 * - VakıfBank kurumsal ekstresi: "Borç (TL)" / "Alacak (TL)" kolonları,
 *   tarih ve saat tek hücrede, "GÖN:" / "ALC:" kısa etiketleri.
 * - Hareketsiz dosya Türkçe iletiyle geri çevrilir; ekran hiçbir adımda
 *   çökmez (hata sınırı kartı görünmez, başlık yerinde).
 *
 * Veri: demo ofisine her koşuda yeni mükellef; Atlas'ın ATL2026000000102
 * faturası (Yıldız Market, 5.400,00, 1 Ekim) türetilir. IBAN'lar sahte,
 * hareketler uydurmadır.
 */
const run = `${Date.now()}`.slice(-6);
const taxpayer = { vkn: uniqueVkn(), title: `Banka Yükleme ${run} A.Ş.` };
const PERIOD = "2026-10";

/** Banka kodlu (00062 Garanti, 00015 VakıfBank), kontrol haneleri geçerli sahte TR IBAN. */
function fakeIban(bankCode: string): string {
  const bban = `${bankCode}0${String(Date.now()).slice(-10)}${String(Math.floor(Math.random() * 1e6)).padStart(6, "0")}`;
  let rem = 0;
  for (const ch of `${bban}292700`) rem = (rem * 10 + Number(ch)) % 97;
  return `TR${String(98 - rem).padStart(2, "0")}${bban}`;
}

const spaced = (iban: string) => iban.replace(/(.{4})/g, "$1 ").trim();

function garantiCsv(iban: string): Buffer {
  return Buffer.from(
    [
      "Garanti BBVA",
      "Hesap Hareketleri",
      `IBAN;${spaced(iban)}`,
      "Tarih Aralığı;01.10.2026 - 31.10.2026",
      "",
      "Tarih;Açıklama;Etiket;Tutar;Bakiye;Dekont No",
      `01.10.2026;FAST Gelen - YILDIZ MARKET LTD STI - Fatura ödemesi ${run};Diğer;5.400,00 TL;15.400,00 TL;G${run}1`,
      `02.10.2026;EFT Ücreti ${run};Banka;-8,50 TL;15.391,50 TL;G${run}2`,
      "",
      "Toplam;;",
      "",
    ].join("\n"),
    "utf8",
  );
}

function vakifCsv(iban: string): Buffer {
  return Buffer.from(
    [
      "T.VAKIFLAR BANKASI T.A.O.",
      "HESAP EKSTRESİ",
      `IBAN;${spaced(iban)}`,
      "",
      "İşlem Tarihi;İşlem No;Açıklama;Borç (TL);Alacak (TL);Bakiye (TL)",
      `03.10.2026 09:12;V${run}1;EFT GDN ALC:KALEM KIRTASIYE LTD STI AÇK:EKIM ALIMI ${run};1.250,00;;8.750,00`,
      `04.10.2026 10:40;V${run}2;EFT MASRAFI ${run};4,25;;8.745,75`,
      "Toplam;;;1.254,25;;",
      "",
    ].join("\n"),
    "utf8",
  );
}

type Tx = { uuid: string; amount: string; match_status: string; counterparty_name: string | null; description: string };

test("Garanti ve VakıfBank ekstresi yüklenir; ekran çökmez, ad açıklamadan okunur, eşleştirme önerisi kendiliğinden gelir", async ({
  browser,
  page,
}) => {
  test.setTimeout(300_000);
  const api = await Api.login(DEMO.owner);
  const engagement = await addConsentedTaxpayer(browser, api, taxpayer, "bankayukleme");
  const base = `/v1/tenant/engagements/${engagement}`;
  const up = await api.upload(`${base}/ubl-uploads`, deriveUbl("atlas-02-ticari-satis.xml", taxpayer.vkn, taxpayer.title));
  expect(up.status(), await up.text()).toBeLessThan(300);
  await waitForUploads(api, engagement, 1);

  const garanti = fakeIban("00062");
  const vakif = fakeIban("00015");
  await api.json(await api.post(`${base}/cash-boxes`, { iban: garanti, name: `Garanti TL ${run}` }));
  await api.json(await api.post(`${base}/cash-boxes`, { iban: vakif, name: `VakıfBank TL ${run}` }));

  await uiLogin(page, DEMO.owner);
  await page.goto(officePath(`/engagements/${engagement}/bank`));
  const heading = page.getByRole("heading", { level: 2, name: "Banka hareketleri" });
  await expect(heading).toBeVisible();
  const input = page.locator('input[type="file"][aria-label="Ekstre yükle"]');
  const preview = page.getByTestId("bank-statement-preview");

  // Garanti BBVA.
  await input.setInputFiles({ name: `garanti-${run}.csv`, mimeType: "text/csv", buffer: garantiCsv(garanti) });
  await expect(preview.getByRole("heading", { name: "Ekstre önizleme" })).toBeVisible();
  await expect(preview).toContainText("banka: Garanti BBVA");
  await preview.getByRole("button", { name: "İçe aktar (2 hareket)" }).click();
  await expect(page.getByText("2 yeni hareket eklendi (banka: Garanti BBVA).")).toBeVisible();
  await expect(page.getByTestId("bolum-hatasi")).toHaveCount(0);

  // VakıfBank.
  await input.setInputFiles({ name: `vakifbank-${run}.csv`, mimeType: "text/csv", buffer: vakifCsv(vakif) });
  await expect(preview.getByRole("heading", { name: "Ekstre önizleme" })).toBeVisible();
  await expect(preview).toContainText("banka: VakıfBank");
  await preview.getByRole("button", { name: "İçe aktar (2 hareket)" }).click();
  await expect(page.getByText("2 yeni hareket eklendi (banka: VakıfBank).")).toBeVisible();
  await expect(page.getByTestId("bolum-hatasi")).toHaveCount(0);
  await expect(heading).toBeVisible();

  // Karşı taraf adı açıklamadan; tahsilat faturaya kendiliğinden öneri.
  const list = async () =>
    (await api.json<{ items: Tx[] }>(await api.get(`${base}/bank-transactions?period=${PERIOD}&limit=20`))).items.filter((x) =>
      x.description.includes(run),
    );
  await expect.poll(async () => (await list()).length).toBe(4);
  const items = await list();
  const collection = items.find((x) => x.amount === "5400.00")!;
  expect(collection.counterparty_name).toBe("YILDIZ MARKET LTD STI");
  expect(collection.match_status).toBe("suggested");
  expect(items.find((x) => x.amount === "1250.00")?.counterparty_name).toBe("KALEM KIRTASIYE LTD STI");

  await page.goto(officePath(`/engagements/${engagement}/bank?kayit=bank_transaction:${collection.uuid}`));
  const panel = page.getByTestId("focused-record").getByTestId("bank-transaction-matching");
  await expect(panel.getByText("Önerildi", { exact: true })).toBeVisible();
  await expect(panel).toContainText("ATL2026000000102");

  // Hareketsiz dosya: Türkçe ileti, ekran yerinde.
  await page.goto(officePath(`/engagements/${engagement}/bank`));
  const empty = Buffer.from(
    ["Garanti BBVA", `IBAN;${garanti}`, "", "Tarih;Açıklama;Etiket;Tutar;Bakiye;Dekont No", "", "Toplam;;", ""].join("\n"),
    "utf8",
  );
  await page.locator('input[type="file"][aria-label="Ekstre yükle"]').setInputFiles({
    name: `bos-${run}.csv`,
    mimeType: "text/csv",
    buffer: empty,
  });
  await expect(page.getByText("Ekstre okunamadı.", { exact: false })).toBeVisible();
  await expect(page.getByTestId("bolum-hatasi")).toHaveCount(0);
  await expect(page.getByRole("heading", { level: 2, name: "Banka hareketleri" })).toBeVisible();
  await api.dispose();
});

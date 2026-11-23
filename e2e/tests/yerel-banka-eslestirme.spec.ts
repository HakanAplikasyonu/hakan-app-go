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
 * Excel / CSV ekstreden fatura eşleştirme önerisi (#780): bankaların çoğu
 * Excel / CSV dökümünde karşı tarafın VKN'sini ya da IBAN'ını vermez; yalnız
 * açıklamada "GONDEREN: …" yazar. Bu ad karşı taraf olarak okunur, tutarı ve
 * tarihi tutan faturası "Önerildi" olur ve hareketin panelinde nedeniyle
 * (tutar birebir aynı, karşı taraf adı benziyor) görünür. Bozuk PDF ekstre
 * Türkçe "Bu PDF okunamadı" iletisiyle geri çevrilir.
 *
 * Veri: demo ofisine her koşuda yeni mükellef; Atlas'ın ATL2026000000102
 * faturası (Yıldız Market, 5.400,00, 1 Ekim) türetilir; Garanti BBVA
 * düzeninde CSV ekstre (VKN'siz tahsilat ve EFT ücreti) API ile yüklenir.
 */
const run = `${Date.now()}`.slice(-6);
const taxpayer = { vkn: uniqueVkn(), title: `Ekstre Eşleştirme ${run} A.Ş.` };
const IBAN = "TR660006200000000001002222"; // Garanti (00062), sahte hesap
const PERIOD = "2026-10";

const csv = [
  "Garanti BBVA",
  "Hesap Hareketleri;",
  `IBAN;${IBAN}`,
  "Dönem;01.10.2026 - 31.10.2026",
  "",
  "Tarih;Açıklama;Etiket;Tutar;Bakiye;Dekont No",
  "01.10.2026;FAST GELEN GONDEREN: YILDIZ MARKET LTD STI FATURA ODEMESI;;5.400,00;15.400,00;E2E001",
  "02.10.2026;EFT UCRETI;;-8,50;15.391,50;E2E002",
  "",
  "Toplam;;",
  "",
].join("\n");

type Tx = {
  uuid: string;
  amount: string;
  match_status: string;
  counterparty_name: string | null;
};
type Matching = {
  match_status: string;
  candidates: {
    invoice_number: string | null;
    parts: Record<string, number>;
  }[];
};

test("CSV ekstre: açıklamadaki gönderen faturaya öneri olur, panelde nedeniyle görünür; bozuk PDF Türkçe hata (#780)", async ({
  browser,
  page,
}) => {
  test.setTimeout(240_000);
  const api = await Api.login(DEMO.owner);
  const engagement = await addConsentedTaxpayer(
    browser,
    api,
    taxpayer,
    "eslestirme",
  );
  const base = `/v1/tenant/engagements/${engagement}`;

  const up = await api.upload(
    `${base}/ubl-uploads`,
    deriveUbl("atlas-02-ticari-satis.xml", taxpayer.vkn, taxpayer.title),
  );
  expect(up.status(), await up.text()).toBeLessThan(300);
  await waitForUploads(api, engagement, 1);

  await api.json(
    await api.post(`${base}/cash-boxes`, { iban: IBAN, name: "Garanti TL" }),
  );
  const report = await api.json<{
    bank_id: string;
    accounts: { inserted: number; errors?: unknown[] }[];
  }>(
    await api.upload(`${base}/bank-statements`, {
      name: "garanti-2026-10.csv",
      mimeType: "text/csv",
      buffer: Buffer.from(csv, "utf8"),
    }),
  );
  expect(report.bank_id).toBe("garanti");
  expect(report.accounts[0].inserted).toBe(2);
  expect(report.accounts[0].errors ?? []).toHaveLength(0);

  const run1 = await api.json<{ suggested: number; auto_matched: number }>(
    await api.post(`${base}/bank-matching?period=${PERIOD}`),
  );
  expect(run1.suggested + run1.auto_matched).toBeGreaterThanOrEqual(1);

  const txs = await api.json<{ items: Tx[] }>(
    await api.get(`${base}/bank-transactions?period=${PERIOD}&limit=10`),
  );
  const collection = txs.items.find((x) => x.amount === "5400.00")!;
  expect(collection).toBeTruthy();
  expect(collection.counterparty_name).toBe("YILDIZ MARKET LTD STI");
  expect(collection.match_status).toBe("suggested");

  const tx = `${base}/bank-transactions/${collection.uuid}`;
  const m = await api.json<Matching>(await api.get(`${tx}/matching`));
  const best = m.candidates.find(
    (c) => c.invoice_number === "ATL2026000000102",
  )!;
  expect(best).toBeTruthy();
  expect(Object.keys(best.parts)).toEqual(
    expect.arrayContaining(["amount_exact", "name"]),
  );

  // Ekran: hareketin panelinde öneri ve nedeni.
  await uiLogin(page, DEMO.owner);
  await page.goto(
    officePath(
      `/engagements/${engagement}/bank?kayit=bank_transaction:${collection.uuid}`,
    ),
  );
  const panel = page
    .getByTestId("focused-record")
    .getByTestId("bank-transaction-matching");
  await expect(panel).toContainText(
    "FAST GELEN GONDEREN: YILDIZ MARKET LTD STI",
  );
  await expect(panel.getByText("Önerildi", { exact: true })).toBeVisible();
  await expect(panel).toContainText("ATL2026000000102");
  const reason = panel.getByTestId("bank-match-reason").first();
  await expect(reason).toContainText("tutar birebir aynı");
  await expect(reason).toContainText("karşı taraf adı benziyor");
  await expect(reason).not.toContainText("Yalnız tutar aynı");

  // Bozuk PDF: Türkçe ileti, hareket eklenmez.
  await page.goto(officePath(`/engagements/${engagement}/bank`));
  await page
    .locator('input[type="file"][aria-label="Ekstre yükle"]')
    .setInputFiles({
      name: "bozuk-ekstre.pdf",
      mimeType: "application/pdf",
      buffer: Buffer.from("%PDF-1.4\nbu dosya bozuk\n%%EOF", "utf8"),
    });
  await expect(
    page.getByText(
      "Bu PDF okunamadı; bankanızın internet şubesinden Excel ya da CSV ekstre indirin.",
    ),
  ).toBeVisible();
  const bad = await api.upload(`${base}/bank-statements`, {
    name: "bozuk-ekstre.pdf",
    mimeType: "application/pdf",
    buffer: Buffer.from("%PDF-1.4\nbu dosya bozuk\n%%EOF", "utf8"),
  });
  expect(bad.status()).toBe(422);
  expect((await bad.json()).error.code).toBe("PDF_UNREADABLE");
  const after = await api.json<{ items: Tx[] }>(
    await api.get(`${base}/bank-transactions?period=${PERIOD}&limit=10`),
  );
  expect(after.items).toHaveLength(2);
  await api.dispose();
});

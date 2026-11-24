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
 * Geç gelen belge ve geçmiş dönem takibi (#780 C): belgenin geliş tarihi ile
 * ait olduğu dönem ayrıdır. Eylül 2026 kilitlendikten sonra Eylül'e iki
 * masraf fişi gelir (Ağustos'a gelen, kapanmamış döneme olduğu için geç
 * sayılmaz). Ofis belgede, dönem şeridinde, mükellefin "Geçmiş dönem"
 * sekmesinde ve ofis geneli listede uyarıyı görür; "Cari döneme al" kararı
 * kaydedilir, geri alınabilir. Stajyer karar veremez.
 *
 * Veri: demo ofisine her koşuda yeni mükellef; masraflar API ile CSV'den.
 */
const run = `${Date.now()}`.slice(-6);
const taxpayer = { vkn: uniqueVkn(), title: `Geç Belge ${run} A.Ş.` };

type LateDocument = {
  uuid: string;
  document_type: string;
  document_uuid: string;
  counterparty: string | null;
  period: string;
  close_reason: string;
  status: string;
  target_period: string | null;
};
type LateList = {
  items: LateDocument[];
  counts: {
    pending: number;
    correction: number;
    current_period: number;
    ignored: number;
  };
};
type Summary = {
  pending: number;
  items: { engagement_uuid: string; pending: number; total: number }[];
};

const CSV =
  "Tarih;Satıcı;VKN / TCKN;Fiş No;Açıklama;Kategori;KDV Oranı;KDV;Toplam;Ödeme;Plaka\n" +
  "29.09.2026;Hızlı Kargo A.Ş.;;7001;Kargo gönderimi E2E;;20;30,00;180,00;Nakit;\n" +
  "30.09.2026;Kırtasiye Dünyası;;7002;Toner E2E;;20;70,00;420,00;Kredi kartı;\n" +
  "28.08.2026;Köşe Lokantası;;7003;Yemek E2E;;10;45,45;500,00;Nakit;\n";

test("kapanıştan sonra gelen belge uyarılır, karar kaydedilir (#780 C)", async ({
  browser,
  page,
}) => {
  test.setTimeout(240_000);
  const api = await Api.login(DEMO.owner);
  const engagement = await addConsentedTaxpayer(
    browser,
    api,
    taxpayer,
    "gecbelge",
  );
  const base = `/v1/tenant/engagements/${engagement}`;

  // Eylül 2026 kapanır, sonra masraflar gelir.
  const lock = await api.post(`${base}/periods/2026-09/lock`, {
    note: "Eylül kapandı",
  });
  expect(lock.status(), await lock.text()).toBe(200);
  const up = await api.upload(`${base}/expenses/import`, {
    name: "gec-masraf.csv",
    mimeType: "text/csv",
    buffer: Buffer.from(CSV, "utf8"),
  });
  expect(up.status(), await up.text()).toBeLessThan(300);

  const list = await api.json<LateList>(await api.get(`${base}/late-documents`));
  expect(list.counts).toEqual({
    pending: 2,
    correction: 0,
    current_period: 0,
    ignored: 0,
  });
  for (const it of list.items) {
    expect(it).toMatchObject({
      document_type: "expense",
      period: "2026-09",
      close_reason: "lock",
      status: "pending",
    });
  }
  const kargo = list.items.find((i) => i.counterparty === "Hızlı Kargo A.Ş.");
  const toner = list.items.find((i) => i.counterparty === "Kırtasiye Dünyası");
  expect(kargo && toner, "iki geç masraf").toBeTruthy();

  // Stajyer görür ama karar veremez.
  const intern = await Api.login(DEMO.intern);
  const denied = await intern.post(
    `${base}/late-documents/${kargo!.uuid}/decision`,
    { status: "ignored" },
  );
  expect(denied.status()).toBe(403);
  await intern.dispose();

  // Mükellefin "Geçmiş dönem" sekmesi: cümle, neden, tek tık karar.
  await uiLogin(page, DEMO.owner);
  await page.goto(officePath(`/engagements/${engagement}/late-documents`));
  const cards = page.getByTestId("late-document");
  await expect(cards).toHaveCount(2);
  const kargoCard = cards.filter({ hasText: "Hızlı Kargo A.Ş." });
  await expect(kargoCard.getByTestId("late-document-sentence")).toContainText(
    "Bu belge dönem kapandıktan sonra geldi (Eylül 2026 kapanışı:",
  );
  await expect(kargoCard).toContainText("Eylül 2026 dönemi kilitlendi");
  await kargoCard.getByRole("button", { name: "Cari döneme al" }).click();
  await expect(page.getByText("Belge cari döneme alındı.")).toBeVisible();
  await expect(cards).toHaveCount(1);
  await page.getByTestId("late-filter-current_period").click();
  await expect(cards).toHaveCount(1);
  await expect(cards.first().getByTestId("late-document-decision")).toContainText(
    "dönemine alındı",
  );

  const after = await api.json<LateList>(
    await api.get(`${base}/late-documents?status=current_period`),
  );
  expect(after.items).toHaveLength(1);
  expect(after.items[0].document_uuid).toBe(kargo!.document_uuid);
  expect(after.items[0].target_period! > "2026-09").toBe(true);

  // Dönem şeridinin altında: Eylül'e kapanıştan sonra gelen 1 bekleyen belge.
  // Özet sayfası ?period= okumaz (dönem üst çubuktan); adresteki dönemi
  // seçiciye işleyen bir sekmeden (Banka) gelinir, şerit ve not ortak düzende.
  await page.goto(officePath(`/engagements/${engagement}/bank?period=2026-09`));
  await expect(page.getByTestId("late-period-note")).toContainText(
    "Eylül 2026 kapandıktan sonra 1 belge geldi",
  );

  // Belgenin kendi ekranında uyarı (seçili masraf).
  await page.goto(
    officePath(
      `/engagements/${engagement}/expenses?kayit=${encodeURIComponent(`expense:${toner!.document_uuid}`)}`,
    ),
  );
  await expect(page.getByTestId("late-document-note")).toContainText(
    "Bu belge dönem kapandıktan sonra geldi",
  );

  // Ofis geneli: mükellefin satırı, bekleyen 1.
  const summary = await api.json<Summary>(
    await api.get("/v1/tenant/late-documents/summary"),
  );
  expect(
    summary.items.find((r) => r.engagement_uuid === engagement),
  ).toMatchObject({ pending: 1, total: 2 });
  await page.goto(officePath("/late-documents"));
  await expect(
    page
      .getByTestId("late-documents-summary")
      .getByRole("row", { name: new RegExp(taxpayer.title) }),
  ).toBeVisible();
  await page.goto(officePath("/tax-returns"));
  await expect(page.getByTestId("late-documents-banner")).toBeVisible();

  // Karar geri alınır.
  const undo = await api.post(
    `${base}/late-documents/${kargo!.uuid}/decision`,
    { status: "pending" },
  );
  expect(undo.status(), await undo.text()).toBe(200);
  const back = await api.json<LateList>(await api.get(`${base}/late-documents`));
  expect(back.counts.pending).toBe(2);

  await api.dispose();
});

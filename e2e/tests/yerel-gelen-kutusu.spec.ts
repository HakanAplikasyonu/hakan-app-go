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
 * Gelen kutusu (#780 b): önerisi olmayan ama müşavirin elini bekleyen her
 * şey İnceleme ekranının "Gelen kutusu" görünümünde tek listede. Demo
 * ofisinde bu ay her türden iş vardır (demo seed: fatura, banka hareketi,
 * masraf fişi, Z raporu, yabancı fatura, geç belge). Yeni mükellefe bu ay
 * bir masraf fişi gelir; mükellefin İnceleme sekmesindeki gelen kutusunda
 * görünür, "İşlemi yap" Masraflar'a gider, "Onayla" satırdan onaylar.
 * Sayaç tıklanınca liste süzülür; Komuta Merkezi'nden bağlantı vardır.
 *
 * Veri: demo ofisine her koşuda yeni mükellef; masraf API ile CSV'den.
 */
const run = `${Date.now()}`.slice(-6);
const taxpayer = { vkn: uniqueVkn(), title: `Gelen Kutusu ${run} A.Ş.` };

const TYPES = [
  "invoice",
  "bank_transaction",
  "expense",
  "retail_sale",
  "foreign_invoice",
  "late_document",
] as const;

type InboxItem = {
  type: (typeof TYPES)[number];
  uuid: string;
  engagement_uuid: string;
  taxpayer_title: string;
  amount: string | null;
  target: { tab: string; id: string };
  can_approve: boolean;
};
type InboxList = {
  period: string;
  items: InboxItem[];
  counts: Record<(typeof TYPES)[number] | "total", number>;
  next_cursor: string | null;
};

/** Bugün, Europe/Istanbul: YYYY-MM-DD. */
const today = new Intl.DateTimeFormat("en-CA", {
  timeZone: "Europe/Istanbul",
  year: "numeric",
  month: "2-digit",
  day: "2-digit",
}).format(new Date());
const period = today.slice(0, 7);
const trDate = `${today.slice(8, 10)}.${today.slice(5, 7)}.${today.slice(0, 4)}`;

const CSV =
  "Tarih;Satıcı;VKN / TCKN;Fiş No;Açıklama;Kategori;KDV Oranı;KDV;Toplam;Ödeme;Plaka\n" +
  `${trDate};Gelen Kutusu Kırtasiye;;8101;Kağıt E2E;;20;40,00;240,00;Nakit;\n`;

test("gelen kutusu: bekleyen her tür tek yerde, masraf satırdan onaylanır (#780 b)", async ({
  browser,
  page,
}) => {
  test.setTimeout(240_000);
  const api = await Api.login(DEMO.owner);

  // Demo ofisi: bu ay her türden bekleyen iş var; liste limit'e uyar.
  const office = await api.json<InboxList>(
    await api.get(`/v1/tenant/inbox?period=${period}&limit=1`),
  );
  expect(office.period).toBe(period);
  for (const k of TYPES) {
    expect(office.counts[k], `demo ofisinde bekleyen ${k}`).toBeGreaterThan(0);
  }
  expect(office.items).toHaveLength(1);
  expect(office.next_cursor).toBe("1");
  const bad = await api.get(`/v1/tenant/inbox?type=suggestion`);
  expect(bad.status()).toBe(400);

  // Yeni mükellefe bu ay bir masraf fişi gelir.
  const engagement = await addConsentedTaxpayer(
    browser,
    api,
    taxpayer,
    "gelenkutusu",
  );
  const up = await api.upload(
    `/v1/tenant/engagements/${engagement}/expenses/import`,
    {
      name: "gelen-kutusu.csv",
      mimeType: "text/csv",
      buffer: Buffer.from(CSV, "utf8"),
    },
  );
  expect(up.status(), await up.text()).toBeLessThan(300);
  const mine = await api.json<InboxList>(
    await api.get(
      `/v1/tenant/inbox?period=${period}&engagement=${engagement}&type=expense`,
    ),
  );
  expect(mine.items).toHaveLength(1);
  expect(mine.items[0]).toMatchObject({
    type: "expense",
    engagement_uuid: engagement,
    taxpayer_title: taxpayer.title,
    amount: "240.00",
    target: { tab: "expenses" },
    can_approve: true,
  });
  expect(mine.counts.expense).toBe(1);

  // Ofis görünümü: "Öneriler | Gelen kutusu" şeridi, sayaç tıklanınca süzer.
  await uiLogin(page, DEMO.owner);
  await page.goto(officePath(`/review?view=inbox&period=${period}`));
  await expect(page.getByTestId("review-mode-inbox")).toHaveAttribute(
    "aria-selected",
    "true",
  );
  const counters = page.getByTestId("inbox-counts");
  for (const label of [
    "Fatura",
    "Banka hareketi",
    "Masraf fişi",
    "Z raporu",
    "Yabancı fatura",
    "Geç belge",
  ]) {
    await expect(counters).toContainText(label);
  }
  await counters.locator('[data-type="retail_sale"]').click();
  await expect(page).toHaveURL(/type=retail_sale/);
  const table = page.getByTestId("inbox-table");
  await expect(table.locator("tbody tr").first()).toBeVisible();
  await expect(table.locator('tbody tr:not([data-type="retail_sale"])')).toHaveCount(0);

  // Mükellefin İnceleme sekmesi: masraf satırı, deep link ve tek tık onay.
  await page.goto(
    officePath(`/engagements/${engagement}/review?view=inbox&period=${period}`),
  );
  const row = page
    .getByTestId("inbox-table")
    .locator('tbody tr[data-type="expense"]');
  await expect(row).toHaveCount(1);
  await expect(row).toContainText("Gelen Kutusu Kırtasiye");
  await expect(row.getByRole("link", { name: "İşlemi yap" })).toHaveAttribute(
    "href",
    new RegExp(`/engagements/${engagement}/expenses`),
  );
  await row.getByRole("button", { name: "Onayla" }).click();
  await expect(page.getByText("Onaylandı.")).toBeVisible();
  await expect(row).toHaveCount(0);
  const after = await api.json<InboxList>(
    await api.get(`/v1/tenant/inbox?period=${period}&engagement=${engagement}`),
  );
  expect(after.counts.expense).toBe(0);

  // Öneriler görünümüne geçiş ve Komuta Merkezi'nden bağlantı.
  await page.getByTestId("review-mode-suggestions").click();
  await expect(page).not.toHaveURL(/view=inbox/);
  await page.goto(officePath("/command"));
  await page.getByRole("link", { name: "Gelen kutusu" }).first().click();
  await expect(page).toHaveURL(/\/review\?view=inbox/);
  await expect(page.getByTestId("inbox")).toBeVisible();

  await api.dispose();
});

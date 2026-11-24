import { expect, test } from "@playwright/test";

import {
  Api,
  DEMO,
  engagementOf,
  expectNoRawText,
  officePath,
  uiLogin,
} from "./lib";

type Item = {
  kind: "bank_statement" | "z_reports" | "expenses" | "purchase_invoices";
  name: string;
  title: string;
  requested: boolean;
};
type Missing = { period: string; items: Item[]; sms_available: boolean };
type Task = { uuid: string; kind: string; title: string; status: string };
type RequestResult = {
  period: string;
  created: Task[];
  existing: Task[];
  sms: { status: string; code?: string } | null;
};

/** Seçilen dönemin bu eksiklere ait açık isteklerini iptal eder. */
async function cancelRequests(
  api: Api,
  engagement: string,
  period: string,
  titles: string[],
) {
  const list = await api.json<{ items: Task[] }>(
    await api.get(
      `/v1/tenant/engagements/${engagement}/document-tasks?status=all&period=${period}&limit=100`,
    ),
  );
  for (const t of list.items) {
    if (
      t.kind === "document_request" &&
      (t.status === "open" || t.status === "submitted") &&
      titles.includes(t.title)
    ) {
      await api.post(
        `/v1/tenant/engagements/${engagement}/document-tasks/${t.uuid}/cancel`,
        { reason: "e2e belge toplama" },
      );
    }
  }
}

/**
 * Ay başı belge toplama (#780, ayın 1–5'i): önceki üç ayda gelen ama geçen
 * ay gelmeyen ekstre, Z raporu, masraf fişi ve alış faturası mükellef
 * bazında görünür; tek tek ya da hepsi birden istenir; açık istek ikinci kez
 * açılmaz. Mükellef özetinde aynı eksikler, panoda ve beyannameler
 * ekranında belge toplamaya bağlantı.
 *
 * Veri: demo ofisi, Selin Yılmaz (demo-seed: iki ay önce dört belge türü,
 * geçen ay hiçbiri). Açılan istekler başta ve sonda iptal edilir.
 */
test("Belge toplama: eksik ekstre, Z, masraf, fatura; tek tık ve toplu istek (#780)", async ({
  page,
}) => {
  const api = await Api.login(DEMO.owner);
  const engagement = await engagementOf(api, DEMO.selin.title);
  const missingUrl = `/v1/tenant/engagements/${engagement}/document-tasks/missing`;

  // Varsayılan dönem geçen ay. Demo dört türü de eksik bırakır; başka
  // testler Selin'e fatura ya da ekstre yükleyebildiği için yalnız Z raporu
  // ve masraf (Selin'e başka yerde girilmez) zorunlu.
  const before = await api.json<Missing>(await api.get(missingUrl));
  const period = before.period;
  expect(period).toMatch(/^\d{4}-\d{2}$/);
  const kinds = before.items.map((i) => i.kind);
  for (const k of ["z_reports", "expenses"] as const) {
    expect(kinds, `${k} eksik görünmeli`).toContain(k);
  }
  const titles = before.items.map((i) => i.title);
  expect(titles.find((x) => x.endsWith("Z raporları"))).toBeTruthy();
  await cancelRequests(api, engagement, period, titles);

  // Geçersiz tür ve hesabı olmayan ekstre reddedilir.
  expect(
    (await api.post(missingUrl, { period, items: [{ kind: "x" }] })).status(),
  ).toBe(400);
  expect(
    (
      await api.post(missingUrl, {
        period,
        items: [{ kind: "bank_statement" }],
      })
    ).status(),
  ).toBe(400);

  // Ekran: mükellef satırı, dört eksik, hiçbiri istenmemiş.
  await page.setViewportSize({ width: 1440, height: 900 });
  await uiLogin(page, DEMO.owner);
  await page.goto(officePath(`/document-collection?period=${period}`));
  const main = page.getByTestId("document-collection-page");
  await expect(
    // Sayfa başlığı (h1); veri gelene dek panel başlığı (h2) da aynı adı taşır.
    main.getByRole("heading", { name: "Ay başı belge toplama", level: 1 }),
  ).toBeVisible();
  const row = main
    .getByTestId("collection-row")
    .filter({ hasText: DEMO.selin.title });
  await expect(row).toHaveCount(1);
  await expect(row.getByTestId("missing-chip")).toHaveCount(
    before.items.length,
  );
  await expect(row.locator('[data-requested="true"]')).toHaveCount(0);
  await expect(main.getByTestId("collection-channel")).toContainText(
    "Uygulama bildirimi ve e-posta gider.",
  );
  await expectNoRawText(main);

  // Tek tık: yalnız Z raporları istenir.
  const z = row.locator('[data-testid="missing-chip"][data-kind="z_reports"]');
  await z.getByRole("button", { name: "İste" }).click();
  await expect(z).toHaveAttribute("data-requested", "true");
  await expect(z).toContainText("İstendi");
  await expect(row.locator('[data-requested="false"]')).toHaveCount(
    before.items.length - 1,
  );

  // Satırdaki "Eksiklerin hepsini iste": kalanlar da istenir.
  await row.getByRole("button", { name: "Eksiklerin hepsini iste" }).click();
  await expect(row.locator('[data-requested="false"]')).toHaveCount(0);
  await expect(row).toContainText("Hepsi istendi");

  // Aynı belge ikinci kez istenmez.
  const again = await api.json<RequestResult>(
    await api.post(missingUrl, { period, sms: true }),
  );
  expect(again.created).toHaveLength(0);
  expect(again.existing.length).toBe(before.items.length);
  expect(again.sms?.status).toBe("skipped");
  const after = await api.json<Missing>(
    await api.get(`${missingUrl}?period=${period}`),
  );
  expect(after.items.every((i) => i.requested)).toBe(true);

  // Mükellef özeti: aynı eksikler, istendi olarak.
  await page.goto(officePath(`/engagements/${engagement}?period=${period}`));
  const panel = page.getByTestId("engagement-missing-documents");
  if (await panel.count()) {
    await expect(panel.locator('[data-requested="true"]')).toHaveCount(
      before.items.length,
    );
  }

  // Beyannameler ekranında belge toplamaya bağlantı.
  await page.goto(officePath("/tax-returns"));
  await expect(page.getByTestId("tax-returns-collection-link")).toHaveAttribute(
    "href",
    /\/document-collection/,
  );

  await cancelRequests(api, engagement, period, titles);
  await api.dispose();
});

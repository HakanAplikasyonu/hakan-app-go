import { expect, test } from "@playwright/test";

import { Api, DEMO, engagementOf, officePath, uiLogin, waitReceiptRead } from "./lib";

/**
 * Canlı demo (#780): mükellef telefondan masraf fişi fotoğrafı yükler
 * (`POST /v1/taxpayer/receipts`, kind=expense_receipt) → yapay zekâ okur
 * (satıcı VKN / unvan, tarih, fiş no, %1 / %10 / %20 matrah + KDV, toplam)
 * → müşavirin Masraflar listesinde sayfa yenilenmeden birkaç saniyede
 * görünür, belgenin tarihinin ayına düşer; müşavir yalnızca hesap kodunu
 * seçer ve onaylar.
 *
 * Veri: demo ofisi, Atlas Gıda (demo mükellef hesabı onun yetkilisi).
 * Yapay zekâ ortamda yoksa (e2e'de AI anahtarı yok, fotoğraf "okunamayan"
 * olur) aynı akış demo seed'in telefondan gelmiş Kasım fişiyle denenir
 * (backend/cmd/demo-seed/livedemo.go). AI sahtesiyle (testfakes.AI) koşarken
 * yüklenen fiş kullanılır.
 */
test.describe.configure({ mode: "serial" });

type Capture = {
  uuid: string;
  status: string;
  draft: { type: string; uuid: string } | null;
};
type Expense = {
  uuid: string;
  period: string | null;
  vendor_name: string | null;
  receipt_number: string | null;
  account_code: string | null;
  account_source: string | null;
  suggested: boolean;
  approval_status: string;
  vat_lines: { tax_rate: string; tax_base: string; tax_amount: string }[];
  review_fields: string[];
};

const SEEDED_RECEIPT = "0457"; // Yıldız Market, 18.11.2026 (demo seed)

/** Ayırt edilebilir, PNG olarak tanınan küçük bir "fotoğraf". */
function photo(tag: string): Buffer {
  return Buffer.concat([
    Buffer.from("\x89PNG\r\n\x1a\n\x00\x00\x00\rIHDR", "latin1"),
    Buffer.from(tag),
  ]);
}

test("mükellefin fiş fotoğrafı müşavirin masraf listesine canlı düşer; hesap seçilir, onaylanır (#780)", async ({
  page,
}) => {
  const office = await Api.login(DEMO.owner);
  const atlas = await engagementOf(office, DEMO.atlas.title);
  const tp = await office.json<{ uuid: string }>(
    await office.get(`/v1/tenant/engagements/${atlas}/taxpayer`),
  );
  const base = `/v1/tenant/engagements/${atlas}`;

  // Müşavir ekranı önce açılır: satır sayfa yenilenmeden gelmelidir.
  await uiLogin(page, DEMO.owner);
  await page.goto(officePath(`/engagements/${atlas}/expenses?status=pending`));
  await expect(page.getByRole("heading", { name: "Masraflar" })).toBeVisible();

  // Mükellef telefondan çeker (mobil uygulamanın kullandığı uç).
  const taxpayer = await Api.login(DEMO.taxpayerUser);
  const res = await taxpayer.postForm(
    "/v1/taxpayer/receipts",
    {
      image: {
        name: "fis.png",
        mimeType: "image/png",
        buffer: photo(`canli-demo-${Date.now()}`),
      },
      kind: "expense_receipt",
    },
    { "X-Taxpayer": tp.uuid, "Idempotency-Key": `canli-demo-${Date.now()}` },
  );
  expect(res.status(), await res.text()).toBe(202);
  // #780: okuma arka planda; sonuç tek fiş ucundan izlenir.
  const accepted = (await res.json()).data as Capture;
  const capture = await waitReceiptRead<Capture>(taxpayer, accepted.uuid, { "X-Taxpayer": tp.uuid });
  await taxpayer.dispose();

  let expenseUuid: string;
  let receiptNo: string;
  const live = capture.status === "draft" && capture.draft?.type === "expense";
  if (live) {
    expenseUuid = capture.draft!.uuid;
    const e = await office.json<Expense>(
      await office.get(`${base}/expenses/${expenseUuid}`),
    );
    receiptNo = e.receipt_number ?? "";
    expect(receiptNo, "fiş no okundu").not.toBe("");
  } else {
    test.info().annotations.push({
      type: "not",
      description: `Yapay zekâ bu ortamda yok (yükleme: ${capture.status}); demo seed'in telefondan gelmiş fişi kullanıldı.`,
    });
    expect(capture.status).toBe("failed");
    // Okunamayan fotoğraf Atlas'ın listesinde kalmasın: ofis kapatır.
    expect(
      (await office.post(`${base}/receipts/${capture.uuid}/reject`)).ok(),
    ).toBeTruthy();
    const page1 = await office.json<{ items: Expense[] }>(
      await office.get(`${base}/expenses?status=pending&limit=200`),
    );
    const seeded = page1.items.find((x) => x.receipt_number === SEEDED_RECEIPT);
    expect(seeded, "demo seed'in Kasım fişi").toBeTruthy();
    expenseUuid = seeded!.uuid;
    receiptNo = SEEDED_RECEIPT;
  }

  // API: oran kırılımı, belge ayı, kontrol edilecek alan, hesap önerisi.
  const e = await office.json<Expense>(
    await office.get(`${base}/expenses/${expenseUuid}`),
  );
  expect(e.approval_status).toBe("pending");
  expect(e.vat_lines.map((l) => l.tax_rate)).toEqual(["1", "10", "20"]);
  expect(e.period).toMatch(/^\d{4}-\d{2}$/);
  expect(e.review_fields, "düşük güvenle okunan satıcı adı").toContain(
    "vendor_name",
  );
  expect(e.suggested).toBe(true);
  expect(e.account_code).toBeTruthy();

  // Ekran: satır yenilemeden gelir (5 sn'de bir sorgu), "Kontrol et" ve öneri görünür.
  const row = page.getByRole("row").filter({ hasText: `#${receiptNo}` });
  await expect(row).toBeVisible({ timeout: 20_000 });
  if (live) await expect(row.getByText("Yeni", { exact: true })).toBeVisible();
  await expect(row.getByText("Kontrol et")).toBeVisible();
  await expect(row.getByText(/%20/).first()).toBeVisible();
  const account = row.getByRole("textbox", { name: "Hesap kodu" });
  await expect(account).toHaveValue(/^[0-9]{3}/);

  // Müşavir yalnızca hesap kodunu seçer ve onaylar.
  const saved = page.waitForResponse(
    (r) =>
      r.url().includes(`/expenses/${expenseUuid}`) &&
      r.request().method() === "PATCH",
  );
  await account.fill("770.01.001");
  await account.press("Enter");
  expect((await saved).status()).toBe(200);
  await expect(row.getByText("Öneri")).toHaveCount(0);
  await row.getByRole("button", { name: /Onayla/ }).click();
  await expect
    .poll(
      async () =>
        (
          await office.json<Expense>(
            await office.get(`${base}/expenses/${expenseUuid}`),
          )
        ).approval_status,
    )
    .toBe("approved");
  const done = await office.json<Expense>(
    await office.get(`${base}/expenses/${expenseUuid}`),
  );
  expect(done.account_code).toBe("770.01.001");
  expect(done.review_fields).toEqual([]);
  await office.dispose();
});

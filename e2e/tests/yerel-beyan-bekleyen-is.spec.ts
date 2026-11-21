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
 * Bekleyen iş beyanı durdurur (#575): dönemde işlenmemiş banka hareketi
 * (eşleşmemiş, yok sayılmamış, karşı hesabı onaylanmamış) her beyan
 * türünün kontrol listesinde "Kontrol ettim" isteyen madde olur; onaylanmadan
 * beyan hazırlanamaz (422). Ofis dönem matrisinde hücre "pending" sayısını
 * taşır ve hazır sayılmaz; dönem şeridi "İşlenmemiş kayıt var" der; ofis
 * Beyannameler ekranının Engel sütunu da bu sayıyı gösterir (birim testi).
 * Kaydı olmayan beyan mükellef ekranında da "Başlanmadı" rozetini taşır.
 *
 * Veri: demo ofisine her koşuda yeni mükellef; Ekim 2026'da iki hareketli
 * MT940 API ile yüklenir; biri API ile 770.01'e işlenir.
 */
const run = `${Date.now()}`.slice(-6);
const taxpayer = { vkn: uniqueVkn(), title: `Bekleyen İş ${run} A.Ş.` };
const PERIOD = "2026-10";

/** Ziraat banka kodlu, kontrol haneleri geçerli, her koşuda farklı TR IBAN. */
function uniqueIban(): string {
  const bban = `000100${String(Date.now()).slice(-10)}${String(Math.floor(Math.random() * 1e6)).padStart(6, "0")}`;
  let rem = 0;
  for (const ch of `${bban}292700`) rem = (rem * 10 + Number(ch)) % 97;
  return `TR${String(98 - rem).padStart(2, "0")}${bban}`;
}

type Item = { code: string; count: number; severity: string };
type Preview = {
  status: string;
  checklist: Item[];
  open_warnings: string[];
  record: unknown;
};
type Cell = {
  period: string;
  ready: boolean;
  pending: {
    bank_transactions: number;
    expenses: number;
    retail_sales: number;
  };
};
type Matrix = {
  total: number;
  items: { engagement_uuid: string; cells: Cell[] }[];
};

test("işlenmemiş banka hareketi kontrol listesinde, matriste ve ofis ekranında görünür (#575)", async ({
  browser,
  page,
}) => {
  test.setTimeout(240_000);
  const api = await Api.login(DEMO.owner);
  const engagement = await addConsentedTaxpayer(
    browser,
    api,
    taxpayer,
    "bekleyen",
  );
  const base = `/v1/tenant/engagements/${engagement}`;

  const iban = uniqueIban();
  await api.json(
    await api.post(`${base}/cash-boxes`, { iban, name: "Ziraat TL" }),
  );
  const statement = [
    ":20:E2E575BEK",
    `:25:${iban}`,
    ":60F:C261001TRY50000,00",
    ":61:2610141014D1200,00NTRFNONREF//B1",
    ":86:KIRTASIYE ODEMESI E2E",
    ":61:2610201020D3400,00NTRFNONREF//B2",
    ":86:EFT GONDERIM E2E",
    ":62F:C261020TRY45400,00",
    "",
  ].join("\r\n");
  const up = await api.upload(`${base}/bank-statements`, {
    name: "bekleyen-2026-10.sta",
    mimeType: "text/plain",
    buffer: Buffer.from(statement, "utf8"),
  });
  expect(up.status(), await up.text()).toBeLessThan(300);
  const txs = (
    await api.json<{ items: { uuid: string; amount: string }[] }>(
      await api.get(`${base}/bank-transactions?period=${PERIOD}&limit=10`),
    )
  ).items;
  expect(txs).toHaveLength(2);

  // Her beyan türünde madde; onaylanmadan hazırlanamaz.
  const preview = async (kind: string) =>
    api.json<Preview>(await api.get(`${base}/tax-returns/${kind}/${PERIOD}`));
  for (const kind of ["kdv1", "muhsgk", "babs"]) {
    const p = await preview(kind);
    expect(
      p.checklist.find((i) => i.code === "PENDING_BANK_TRANSACTIONS"),
      kind,
    ).toMatchObject({ count: 2, severity: "warning" });
    expect(p.open_warnings, kind).toContain("PENDING_BANK_TRANSACTIONS");
  }
  const kdv1 = await preview("kdv1");
  const blocked = await api.post(`${base}/tax-returns/kdv1/${PERIOD}/prepare`, {
    acknowledged: kdv1.open_warnings.filter(
      (c) => c !== "PENDING_BANK_TRANSACTIONS",
    ),
  });
  expect(blocked.status()).toBe(422);
  expect(await blocked.text()).toContain("PENDING_BANK_TRANSACTIONS");

  // Biri hesaba işlenince sayı düşer.
  const booked = await api.put(`${base}/bank-postings/${txs[0].uuid}`, {
    account_code: "770.01",
  });
  expect(booked.status(), await booked.text()).toBe(204);
  expect(
    (await preview("kdv1")).checklist.find(
      (i) => i.code === "PENDING_BANK_TRANSACTIONS",
    ),
  ).toMatchObject({ count: 1 });

  // Ofis matrisi: bekleyen iş sayılır, hücre hazır değil.
  let cell: Cell | undefined;
  for (let offset = 0; !cell; offset += 100) {
    const m = await api.json<Matrix>(
      await api.get(
        `/v1/tenant/periods/matrix?until=${PERIOD}&periods=1&limit=100&offset=${offset}`,
      ),
    );
    cell = m.items
      .find((r) => r.engagement_uuid === engagement)
      ?.cells.find((c) => c.period === PERIOD);
    if (!cell && offset + 100 >= m.total) break;
  }
  expect(cell, "matriste mükellefin Ekim hücresi").toBeTruthy();
  expect(cell!.pending).toEqual({
    bank_transactions: 1,
    expenses: 0,
    retail_sales: 0,
  });
  expect(cell!.ready).toBe(false);

  // Mükellef ekranı: madde ilgili kayıtlara (banka) gider; MUHSGK "Başlanmadı".
  await uiLogin(page, DEMO.owner);
  await page.goto(
    officePath(`/engagements/${engagement}/tax-returns?period=${PERIOD}`),
  );
  const checklist = page.getByRole("region", { name: "Kontrol listesi" });
  await expect(checklist).toContainText("1 banka hareketi işlenmedi");
  await expect(
    page.getByRole("region", { name: "Dönem şeridi" }),
  ).toContainText("İşlenmemiş kayıt var");
  await page.goto(
    officePath(
      `/engagements/${engagement}/tax-returns?period=${PERIOD}&kind=muhsgk`,
    ),
  );
  await expect(page.getByText("Başlanmadı").first()).toBeVisible();

  await api.dispose();
});

import { randomUUID } from "node:crypto";
import { readFileSync } from "node:fs";
import path from "node:path";

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
 * Faturadan fiş (#669, Oğuz kararı): uygulama alış / satış faturasının
 * muhasebe fişini üretir; fiş kontrol → onay → aktarım akışına girer.
 *
 *   Satış (atlas-01, 01.10.2026): 120 B 492 / 600.01 A 420 / 391 A 72.
 *   Alış (selin-01, 01.10.2026): 153.01 B 420 / 191 B 72 / 320 A 492.
 *   Geç gelen alış (selin-02): KDV'si Kasım'a alınır → fiş 01.11.2026,
 *   Kasım fişi; belge tarihi satırlarda 01.10.2026 kalır.
 *
 * Faturalar ekranındaki "Ekim 2026 faturalarından fiş oluştur" iki fiş
 * üretir; ikinci basış yeni fiş üretmez. Onaylanan fişler Luca dosyasına
 * girer (belge izi faturaların numarasını taşır).
 *
 * Veri: demo ofisine her koşuda yeni bir mükellef; faturalar ona türetilir.
 */
test.describe.configure({ mode: "serial" });

const run = `${Date.now()}`.slice(-6);
const taxpayer = { vkn: uniqueVkn(), title: `Fiş Fatura ${run} A.Ş.` };
const supplier = uniqueVkn();

type Suggestion = {
  uuid: string;
  status: string;
  target_type: string;
  target_field: string;
  proposed_value: unknown;
};
type Invoice = {
  uuid: string;
  invoice_number: string;
  direction: string;
  payable_amount: string;
};
type DraftRef = {
  uuid: string;
  entry_no: number;
  period: string;
  status: string;
};
type DraftLine = {
  account: string;
  debit: string;
  credit: string;
  document_no?: string;
  document_date?: string;
  source_kind?: string;
};
type Draft = {
  uuid: string;
  status: string;
  period: string;
  entry_date: string;
  items: DraftLine[];
};

/** Selin'in alış faturası mükellefe: alıcı mükellef (VKN), satıcı başka bir VKN. */
function purchaseUbl(file: string) {
  let xml = readFileSync(path.join(DEMO.ublDir, file), "utf8");
  const ettn = xml.match(/<(?:cbc:)?UUID\b[^>]*>([^<]+)<\/(?:cbc:)?UUID>/)?.[1];
  if (!ettn) throw new Error(`no ETTN in ${file}`);
  xml = xml
    .split(`schemeID="TCKN">${DEMO.selin.taxId}`)
    .join(`schemeID="VKN">${taxpayer.vkn}`)
    .split(DEMO.atlas.taxId)
    .join(supplier)
    .split(ettn)
    .join(randomUUID().toUpperCase());
  return {
    name: file.replace("selin-", "e2e-"),
    mimeType: "application/xml",
    buffer: Buffer.from(xml, "utf8"),
  };
}

const num = (s: string | undefined) => Number(s ?? 0);

test.describe("faturadan fiş (#669)", () => {
  let api: Api;
  let engagement = "";
  const base = () => `/v1/tenant/engagements/${engagement}`;
  let sale: Invoice;
  let purchase: Invoice;
  let late: Invoice;

  /**
   * Satır hesaplarını onaylar: faturanın satır hesabı önerileri (bekleyen ya
   * da otomatik uygulanmış) verilen koda düzeltilir.
   */
  async function approveLineAccounts(invoice: string, code: string) {
    await api.post(`${base()}/invoices/${invoice}/account-suggestions`, {});
    const lines = (
      await api.json<{ items: Suggestion[] }>(
        await api.get(
          `${base()}/suggestions?status=all&kind=account_code&invoice=${invoice}&limit=100`,
        ),
      )
    ).items.filter(
      (s) =>
        s.target_type === "invoice_item" &&
        s.target_field === "account_code" &&
        !["superseded", "reverted", "rejected"].includes(s.status),
    );
    expect(lines.length, `line suggestions of ${invoice}`).toBeGreaterThan(0);
    for (const s of lines) {
      const value: Record<string, unknown> = {
        ...(s.proposed_value as Record<string, unknown>),
        account_code: code,
      };
      delete value.account_name;
      delete value.open_new;
      const res = await api.post(`${base()}/suggestions/${s.uuid}/correct`, {
        value,
        reason: "e2e faturadan fiş",
      });
      expect(res.status(), await res.text()).toBeLessThan(300);
    }
  }

  async function draftOf(invoice: string) {
    const link = await api.json<{ draft: DraftRef | null }>(
      await api.get(`${base()}/invoices/${invoice}/journal-draft`),
    );
    expect(link.draft, `draft of ${invoice}`).not.toBeNull();
    return api.json<Draft>(
      await api.get(`${base()}/journal-drafts/${link.draft!.uuid}`),
    );
  }

  /**
   * Ana hesap başına borç / alacak toplamı (KDV ve cari alt hesabı plana ve
   * otomatik önerilere göre değişebilir); satır hesabı tam kodla denetlenir.
   */
  function expectEntry(
    d: Draft,
    want: Record<string, [number, number]>,
    line: string,
    number: string,
  ) {
    const sums: Record<string, [number, number]> = {};
    for (const l of d.items) {
      const main = l.account.slice(0, 3);
      const [b, a] = sums[main] ?? [0, 0];
      sums[main] = [
        Math.round((b + num(l.debit)) * 100) / 100,
        Math.round((a + num(l.credit)) * 100) / 100,
      ];
      expect(l).toMatchObject({ document_no: number, source_kind: "invoice" });
      expect(l.document_date?.slice(0, 10)).toBe("2026-10-01");
    }
    expect(sums).toEqual(want);
    const main = line.slice(0, 3);
    expect(
      d.items.filter((l) => l.account.startsWith(main)).map((l) => l.account),
    ).toEqual([line]);
  }

  test.beforeAll(async ({ browser }) => {
    test.setTimeout(300_000);
    api = await Api.login(DEMO.owner);
    engagement = await addConsentedTaxpayer(browser, api, taxpayer, "fisfat");
    for (const file of [
      deriveUbl("atlas-01-temel-satis.xml", taxpayer.vkn, taxpayer.title),
      purchaseUbl("selin-01-temel-alis.xml"),
      purchaseUbl("selin-02-earsiv-alis.xml"),
    ]) {
      const up = await api.upload(`${base()}/ubl-uploads`, file);
      expect(up.status(), await up.text()).toBeLessThan(300);
    }
    await waitForUploads(api, engagement, 3);
    const items = (
      await api.json<{ items: Invoice[] }>(
        await api.get(`${base()}/invoices?limit=10`),
      )
    ).items;
    sale = items.find((i) => i.direction === "outgoing")!;
    purchase = items.find(
      (i) => i.direction === "incoming" && num(i.payable_amount) === 492,
    )!;
    late = items.find(
      (i) => i.direction === "incoming" && i.uuid !== purchase?.uuid,
    )!;
    expect([sale, purchase, late].every(Boolean), JSON.stringify(items)).toBe(
      true,
    );

    await approveLineAccounts(sale.uuid, "600.01");
    await approveLineAccounts(purchase.uuid, "153.01");
    await approveLineAccounts(late.uuid, "153.01");
    // Geç gelen alışın KDV'si Kasım'a (#640).
    const moved = await api.put(
      `${base()}/invoices/${late.uuid}/vat-deduction-period`,
      { period: "2026-11" },
    );
    expect(moved.status(), await moved.text()).toBe(200);
  });

  test.afterAll(async () => {
    await api?.dispose();
  });

  test("Ekim faturalarından fiş: satış ve alış fişi, ikinci basış yeni fiş üretmez", async ({
    page,
  }) => {
    await uiLogin(page, DEMO.owner);
    await page.goto(
      officePath(`/invoices?taxpayer=${engagement}&period=2026-10`),
    );
    const run = page.getByRole("button", {
      name: /^Ekim 2026 faturalarından fiş oluştur/,
    });
    await run.click();
    const result = page.getByTestId("invoice-posting-result");
    await expect(result).toContainText(
      "2 fiş oluşturuldu; 0 fatura fişlenemedi",
    );
    await expect(result.getByTestId("invoice-posting-created")).toHaveCount(2);
    await expect(result).toContainText(sale.invoice_number);
    await expect(result).toContainText(purchase.invoice_number);

    // İkinci basış: fişi olmayan fatura kalmadı (geç gelen alış Kasım'da).
    await run.click();
    await expect(result).toContainText(
      "Ekim 2026 için fişi olmayan fatura yok.",
    );

    const s = await draftOf(sale.uuid);
    expect(s).toMatchObject({ status: "draft", period: "2026-10" });
    expect(s.entry_date.slice(0, 10)).toBe("2026-10-01");
    expectEntry(
      s,
      { "120": [492, 0], "600": [0, 420], "391": [0, 72] },
      "600.01",
      sale.invoice_number,
    );

    const p = await draftOf(purchase.uuid);
    expect(p).toMatchObject({ status: "draft", period: "2026-10" });
    expectEntry(
      p,
      { "153": [420, 0], "191": [72, 0], "320": [0, 492] },
      "153.01",
      purchase.invoice_number,
    );

    // Tekil istek de aynı fişi döner.
    const again = await api.json<{ outcome: string; draft: DraftRef }>(
      await api.post(`${base()}/invoices/${sale.uuid}/journal-draft`, {}),
    );
    expect(again).toMatchObject({
      outcome: "existing",
      draft: { uuid: s.uuid },
    });
  });

  test("geç gelen alış: fiş KDV indirim ayının ilk günü, belge tarihi korunur", async () => {
    const nov = await api.json<{
      created: number;
      items: { invoice_uuid: string; outcome: string }[];
    }>(await api.post(`${base()}/invoice-postings`, { period: "2026-11" }));
    expect(nov.created).toBe(1);
    expect(nov.items[0]).toMatchObject({
      invoice_uuid: late.uuid,
      outcome: "created",
    });
    const d = await draftOf(late.uuid);
    expect(d).toMatchObject({ status: "draft", period: "2026-11" });
    expect(d.entry_date.slice(0, 10)).toBe("2026-11-01");
    const total = num(late.payable_amount);
    const debit = d.items.reduce((a, l) => a + num(l.debit), 0);
    const credit = d.items.reduce((a, l) => a + num(l.credit), 0);
    expect(Math.round(debit * 100)).toBe(Math.round(credit * 100));
    expect(d.items.at(-1)!.account).toMatch(/^320\b/);
    expect(num(d.items.at(-1)!.credit)).toBeCloseTo(total, 2);
    for (const l of d.items)
      expect(l.document_date?.slice(0, 10)).toBe("2026-10-01");
  });

  test("onaylanan fatura fişleri Luca dosyasına girer", async () => {
    const s = await draftOf(sale.uuid);
    const p = await draftOf(purchase.uuid);
    for (const d of [s, p]) {
      const ok = await api.post(
        `${base()}/journal-drafts/${d.uuid}/approve`,
        {},
      );
      expect(ok.status(), await ok.text()).toBeLessThan(300);
    }
    const batch = await api.json<{ uuid: string; files: unknown[] }>(
      await api.post(`${base()}/journal-exports`, {
        driver: "luca",
        drafts: [s.uuid, p.uuid],
      }),
    );
    expect(batch.files.length).toBeGreaterThan(0);
    const docs = (
      await api.json<{
        items: {
          document_no?: string;
          journal_draft_uuid: string;
          file_name: string;
        }[];
      }>(
        await api.get(
          `${base()}/journal-export-documents?batch=${batch.uuid}&limit=50`,
        ),
      )
    ).items;
    expect(docs.map((x) => x.document_no).sort()).toEqual(
      [sale.invoice_number, purchase.invoice_number].sort(),
    );
    expect((await draftOf(sale.uuid)).status).toBe("exported");
  });
});

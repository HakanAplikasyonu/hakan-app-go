import { readFileSync } from "node:fs";

import { expect, test } from "@playwright/test";

import { addConsentedTaxpayer, Api, DEMO, officePath, uiLogin, uniqueVkn } from "./lib";

/**
 * #669 G Banka hareketi fiş satırı / cari hareketiyle eşleşir: faturası
 * olmayan tahsilat, daha önce girilmiş bir fişin cari satırına bağlanır;
 * "Faturası olmayan tahsilat" uyarısı çözülür. Başka bir satırla yeniden
 * eşleştirme tek adımdır (önce geri almak gerekmez) ve geçmişe "Yeniden
 * eşleştirdi" olarak, neyi değiştirdiğiyle düşer.
 *
 * Veri: demo ofisine her koşuda yeni bir mükellef; Ziraat ekstresi
 * (fixtures/atlas-ziraat-2026-10.sta, 5.400,00 gelen EFT) ve bu dosyanın
 * kendi iki fişi. Fatura yüklenmez. Başka dosyanın kaydına dokunmaz.
 */
test.describe.configure({ mode: "serial" });

const run = `${Date.now()}`.slice(-6);
const taxpayer = { vkn: uniqueVkn(), title: `Fiş Eşleşme ${run} A.Ş.` };
const PERIOD = "2026-10";
const UNINVOICED = "BNK_UNINVOICED_COLLECTION";
const CARI = "120.01.001";
const OTHER = "649.01";

type Matching = {
  match_status: string;
  line_matchable: boolean;
  journal_matches: { account_code: string; amount: string; label: string }[];
};
type HistoryEntry = {
  action: string;
  journal_lines: string[];
  replaced: string[];
  actor_name: string | null;
};

test.describe("#669 G banka hareketi ↔ fiş satırı", () => {
  let engagement = "";
  let collection = "";
  let api: Api;

  const base = () => `/v1/tenant/engagements/${engagement}`;
  const tx = () => `${base()}/bank-transactions/${collection}`;
  const openUninvoiced = async () =>
    (
      await api.json<{ total: number }>(
        await api.get(`${base()}/warnings?status=open&code=${UNINVOICED}&period=${PERIOD}&limit=5`),
      )
    ).total;

  test.beforeAll(async ({ browser }) => {
    test.setTimeout(240_000);
    api = await Api.login(DEMO.owner);
    engagement = await addConsentedTaxpayer(browser, api, taxpayer, "fiseslesme");

    await api.json(await api.post(`${base()}/cash-boxes`, { iban: "TR76 0001 0005 1978 6457 8413 26", name: "Ziraat TL" }));
    await api.json(
      await api.upload(`${base()}/bank-statements`, {
        name: "atlas-ziraat-2026-10.sta",
        mimeType: "text/plain",
        buffer: readFileSync("fixtures/atlas-ziraat-2026-10.sta"),
      }),
    );
    const res = await api.json<{ findings: string[] }>(await api.post(`${base()}/bank-matching?period=${PERIOD}`));
    expect(res.findings).toContain(UNINVOICED);

    const txs = await api.json<{ items: { uuid: string; amount: string }[] }>(
      await api.get(`${base()}/bank-transactions?period=${PERIOD}&limit=10`),
    );
    collection = txs.items.find((x) => x.amount === "5400.00")!.uuid;

    // İki elle fiş: tahsilat cariye (120) ve başka bir hesaba (649).
    for (const [account, note] of [
      [CARI, `Yıldız Market tahsilatı ${run}`],
      [OTHER, `Diğer gelir ${run}`],
    ]) {
      await api.json(
        await api.post(`${base()}/journal-drafts`, {
          entry_type: "mahsup",
          entry_date: `${PERIOD}-04T00:00:00Z`,
          description: note,
          lines: [
            { account: "100.01", description: "Kasa", debit: "5400.00", credit: "0" },
            { account, description: note, debit: "0", credit: "5400.00" },
          ],
        }),
      );
    }
    expect(await openUninvoiced()).toBe(1);
  });

  test.afterAll(async () => {
    await api?.dispose();
  });

  test("cari satırıyla eşleştir → başka satırla tek adımda yeniden eşleştir", async ({ page }) => {
    await uiLogin(page, DEMO.owner);
    await page.goto(officePath(`/engagements/${engagement}/bank?kayit=bank_transaction:${collection}`));
    const panel = page.getByTestId("focused-record").getByTestId("bank-transaction-matching");
    const lines = panel.getByTestId("bank-transaction-lines");
    const log = panel.getByTestId("bank-transaction-history");

    // Tutarı tutan iki satır önerilir; yalnız cari hesaplarda 649 düşer.
    await expect(lines).toContainText("Fiş satırı ya da cari hareketiyle eşleştir");
    const cariBox = lines.getByRole("checkbox", { name: new RegExp(`${CARI.replace(/\./g, "\\.")} satırını seç`) });
    const otherBox = lines.getByRole("checkbox", { name: new RegExp(`${OTHER.replace(/\./g, "\\.")} satırını seç`) });
    await expect(cariBox).toBeVisible();
    await expect(otherBox).toBeVisible();
    // Bankadan üretilmemiş ama ters taraftaki (borç) kasa satırı aday değil.
    await expect(lines).not.toContainText("100.01");
    await lines.getByLabel("Yalnız cari hesaplar").check();
    await expect(otherBox).toHaveCount(0);
    await expect(cariBox).toBeVisible();

    // Eşleştir: uyarı çözülür, geçmişe düşer.
    await cariBox.check();
    await panel.getByRole("button", { name: "Seçilenlerle eşleştir (1)" }).click();
    const matched = panel.getByTestId("bank-transaction-line-matches");
    await expect(matched).toContainText(CARI);
    await expect(panel.getByText("Eşleşti", { exact: true })).toBeVisible();
    await expect(log.getByRole("listitem")).toHaveCount(1);
    await expect(log.getByRole("listitem").first()).toContainText("Fiş satırına eşleştirdi");
    await expect(log.getByRole("listitem").first()).toContainText(CARI);
    await expect.poll(openUninvoiced).toBe(0);

    // Yeniden eşleştir: geri almadan başka satırı seç.
    await lines.getByLabel("Yalnız cari hesaplar").uncheck();
    await otherBox.check();
    await expect(panel).toContainText("Eski eşleşme kaldırılır, seçilenlerle değiştirilir.");
    await panel.getByRole("button", { name: "Seçilenlerle yeniden eşleştir (1)" }).click();
    await expect(matched).toContainText(OTHER);
    await expect(matched).not.toContainText(CARI);
    await expect(log.getByRole("listitem")).toHaveCount(2);
    await expect(log.getByRole("listitem").first()).toContainText("Yeniden eşleştirdi");
    await expect(log.getByRole("listitem").first()).toContainText(`önceki:`);
    await expect(log).not.toContainText("Bilinmeyen kullanıcı");

    // API: tek eşleşme, geçmiş en yenisi önce, neyi değiştirdiği kayıtlı.
    const m = await api.json<Matching>(await api.get(`${tx()}/matching`));
    expect(m.match_status).toBe("matched");
    expect(m.line_matchable).toBe(true);
    expect(m.journal_matches.map((x) => x.account_code)).toEqual([OTHER]);
    expect(m.journal_matches[0].amount).toBe("5400.00");
    const h = (await api.json<{ items: HistoryEntry[] }>(await api.get(`${tx()}/history`))).items;
    expect(h.map((e) => e.action)).toEqual(["rematched", "matched"]);
    expect(h[0].journal_lines.join()).toContain(OTHER);
    expect(h[0].replaced.join()).toContain(CARI);
    expect(h[1].journal_lines.join()).toContain(CARI);
    for (const e of h) expect(e.actor_name).toBeTruthy();
    expect(await openUninvoiced()).toBe(0);

    // Bilinmeyen fiş satırı reddedilir; eşleşme yerinde kalır.
    const bad = await api.put(`${tx()}/matches`, {
      journal_lines: [{ journal_draft_uuid: "00000000-0000-4000-8000-000000000000", line_no: 1 }],
    });
    expect(bad.status()).toBe(422);
    const after = await api.json<Matching>(await api.get(`${tx()}/matching`));
    expect(after.journal_matches.map((x) => x.account_code)).toEqual([OTHER]);

    // Sayfa yenilense de eşleşme ve geçmiş aynı.
    await page.reload();
    await expect(panel.getByTestId("bank-transaction-line-matches")).toContainText(OTHER);
    await expect(log.getByRole("listitem")).toHaveCount(2);
  });
});

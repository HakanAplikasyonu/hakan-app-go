import { expect, test } from "@playwright/test";

import { addConsentedTaxpayer, Api, DEMO, officePath, uiLogin, uniqueVkn } from "./lib";

/**
 * Banka bakiyesi ile muhasebe (102) bakiyesi farkı (#780 C).
 *
 * - Ekstre bakiyesi 102.01 bakiyesinden farklı; fark iki nedene ayrılır:
 *   fişe girmemiş bir banka hareketi (−100) ve banka hareketi olmayan elle
 *   bir 102.01 satırı (+40 fazla: neden −40). Her neden tutarıyla ve kaydın
 *   bağlantısıyla görünür.
 * - Ekran: banka sayfasının "Muhasebe farkı" sekmesinde nedenler; bir neden
 *   "açıklandı" işaretlenir (kim / ne zaman yazar), diğerine düzeltme fişi
 *   kesilir (689 karşı hesap). Açık fark sıfırlanır.
 * - API: açıklandı işareti geri alınır.
 *
 * Veri: demo ofisine her koşuda yeni bir mükellef; MT940 ekstresi
 * (1–6 Ekim: 1.000 → 1.400), açılış fişi 1.000, iki hareketten biri fişli.
 */
const run = `${Date.now()}`.slice(-6);
const taxpayer = { vkn: uniqueVkn(), title: `Bakiye Fark ${run} A.Ş.` };
const PERIOD = "2026-10";
const IBAN = "TR760001000519786457841326";

const statement = Buffer.from(
  [
    `:20:FARK${run}`,
    `:25:${IBAN}`,
    ":60F:C261001TRY1000,00",
    ":61:2610021002C500,00NTRFNONREF//F1",
    ":86:HAVALE GELEN FARK MUSTERI",
    ":61:2610061006D100,00NTRFNONREF//F2",
    ":86:EFT GIDEN FARK TEDARIKCI",
    ":62F:C261006TRY1400,00",
  ].join("\n") + "\n",
  "utf8",
);

type Reason = {
  reason: string;
  amount: string;
  corrected: string;
  open: string;
  explained: { uuid: string; created_by: string } | null;
  records: { kind: string; transaction_uuid?: string; draft_uuid?: string }[];
};
type Report = {
  items: {
    cash_box_uuid: string;
    account_code: string;
    statement_balance: string | null;
    ledger_balance: string;
    difference: string | null;
    open_total: string;
    reasons: Reason[];
  }[];
};

test("banka ile 102 farkı nedenlere ayrılır; açıklanır ve düzeltme fişi kesilir (#780)", async ({ browser, page }) => {
  test.setTimeout(240_000);
  const api = await Api.login(DEMO.owner);
  const engagement = await addConsentedTaxpayer(browser, api, taxpayer, "fark");
  const base = `/v1/tenant/engagements/${engagement}`;
  const box = await api.json<{ uuid: string }>(
    await api.post(`${base}/cash-boxes`, { iban: "TR76 0001 0005 1978 6457 8413 26", name: "Ziraat TL" }),
  );
  expect((await api.put(`${base}/bank-ledger-accounts/${box.uuid}`, { account_code: "102.01" })).status()).toBeLessThan(300);
  await api.json(await api.upload(`${base}/bank-statements`, { name: `fark-${run}.sta`, mimeType: "text/plain", buffer: statement }));

  // Açılış fişi ve gelen havalenin fişi; giden EFT fişsiz kalır.
  await api.json(
    await api.post(`${base}/journal-drafts`, {
      entry_type: "acilis",
      entry_date: "2026-01-01T00:00:00Z",
      description: "Açılış",
      lines: [
        { account: "102.01", debit: "1000", credit: "0" },
        { account: "500", debit: "0", credit: "1000" },
      ],
    }),
  );
  const txs = await api.json<{ items: { uuid: string; amount: string }[] }>(
    await api.get(`${base}/bank-transactions?period=${PERIOD}&limit=10`),
  );
  const incoming = txs.items.find((x) => x.amount === "500.00")!;
  const outgoing = txs.items.find((x) => x.amount === "100.00")!;
  expect(incoming && outgoing).toBeTruthy();
  const set = await api.put(`${base}/bank-postings/${incoming.uuid}`, { account_code: "120.01" });
  expect(set.status(), await set.text()).toBe(204);
  await api.json(await api.post(`${base}/bank-postings/drafts`, { period: PERIOD }));
  // Banka hareketi olmayan elle 102.01 satırı.
  await api.json(
    await api.post(`${base}/journal-drafts`, {
      entry_type: "mahsup",
      entry_date: `${PERIOD}-10T00:00:00Z`,
      description: "Elle banka girişi",
      lines: [
        { account: "102.01", debit: "40", credit: "0" },
        { account: "100", debit: "0", credit: "40" },
      ],
    }),
  );

  const report = async () => api.json<Report>(await api.get(`${base}/bank-ledger-difference?period=${PERIOD}`));
  const first = (await report()).items.find((a) => a.cash_box_uuid === box.uuid)!;
  // Ekstre 1.400; muhasebe 1.000 + 500 + 40 = 1.540.
  expect(first).toMatchObject({ account_code: "102.01", statement_balance: "1400.00", ledger_balance: "1540.00", difference: "-140.00" });
  const byReason = (a: Report["items"][number]) => Object.fromEntries(a.reasons.map((r) => [r.reason, r]));
  const r1 = byReason(first);
  expect(Object.keys(r1).sort()).toEqual(["manual_lines", "unbooked"]);
  expect(r1.unbooked.amount).toBe("-100.00");
  expect(r1.unbooked.records[0]).toMatchObject({ kind: "bank_transaction", transaction_uuid: outgoing.uuid });
  expect(r1.manual_lines.amount).toBe("-40.00");
  expect(r1.manual_lines.records[0].kind).toBe("journal_line");

  // Ekran: Muhasebe farkı sekmesi.
  await uiLogin(page, DEMO.owner);
  await page.goto(officePath(`/engagements/${engagement}/bank?view=difference&period=${PERIOD}`));
  const panel = page.getByTestId("bank-ledger-difference");
  await expect(panel.getByRole("heading", { name: "Banka bakiyesi ile muhasebe farkı" })).toBeVisible();
  const account = panel.getByTestId("difference-account").filter({ hasText: "102.01" });
  await expect(account).toContainText("Fark var");
  const unbooked = account.locator('[data-reason="unbooked"]');
  await expect(unbooked).toContainText("Fişe girmemiş banka hareketleri");
  await expect(unbooked.getByRole("link", { name: "Hareketi aç" })).toHaveAttribute("href", new RegExp(outgoing.uuid));
  const manual = account.locator('[data-reason="manual_lines"]');
  await expect(manual).toContainText("Banka hareketi olmayan elle fiş satırları");
  await expect(manual.getByRole("link", { name: /Fiş \d+/ })).toBeVisible();

  // Açıklandı: kim, ne zaman.
  await unbooked.getByRole("button", { name: "Açıklandı olarak işaretle" }).click();
  const dialog = page.getByTestId("difference-dialog");
  await dialog.getByLabel("Açıklama").fill("Kasım ekstresiyle fişlenecek");
  await dialog.getByRole("button", { name: "İşaretle" }).click();
  await expect(unbooked.getByTestId("difference-resolution")).toContainText("Açıklandı");
  await expect(unbooked.getByTestId("difference-resolution")).toContainText("Kasım ekstresiyle fişlenecek");

  // Düzeltme fişi: karşı hesap 689, tutar açık kısım (−40).
  await manual.getByRole("button", { name: "Düzeltme fişi kes" }).click();
  await expect(dialog.getByLabel("Karşı hesap")).toHaveValue("689");
  await expect(dialog.getByLabel("Tutar (TL)")).toHaveValue("-40.00");
  await dialog.getByLabel("Açıklama").fill("Yanlış banka girişi");
  await dialog.getByRole("button", { name: "Fiş taslağını oluştur" }).click();
  await expect(manual.getByTestId("difference-resolution")).toContainText("Düzeltme fişi");
  await expect(manual.getByRole("link", { name: "Fişi aç" })).toBeVisible();
  await expect(account).toContainText("Fark açıklandı");

  const after = (await report()).items.find((a) => a.cash_box_uuid === box.uuid)!;
  const r2 = byReason(after);
  expect(after.difference).toBe("-100.00");
  expect(after.open_total).toBe("0.00");
  expect(r2.manual_lines).toMatchObject({ amount: "-40.00", corrected: "-40.00", open: "0.00" });
  expect(r2.unbooked.explained).toBeTruthy();

  // Açıklandı işareti geri alınır; ikinci kez alınamaz.
  const id = r2.unbooked.explained!.uuid;
  expect((await api.delete(`${base}/bank-ledger-difference/resolutions/${id}`)).status()).toBe(204);
  expect((await api.delete(`${base}/bank-ledger-difference/resolutions/${id}`)).status()).toBe(404);
  expect((await report()).items.find((a) => a.cash_box_uuid === box.uuid)!.open_total).toBe("-100.00");
  await api.dispose();
});

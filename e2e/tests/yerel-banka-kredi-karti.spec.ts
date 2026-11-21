import { expect, test } from "@playwright/test";

import { Api, DEMO, engagementOf, officePath, uiLogin } from "./lib";

/**
 * Kart ekstresi ödemesini kredi kartı harcamasına bağlama (#565):
 * entegrasyondan (Easytrade) gelen kredi kartıyla ödenmiş gider, kart
 * ekstresini ödeyen banka hareketinin panelinde "Çek, senet ya da kart
 * ödemesine bağla" bölümünde listelenir; seçilen harcamaya bağlanır, panel
 * "Kapattığı çek, senet ya da kart ödemesi"ni gösterir.
 *
 * Veri: demo ofisi, Atlas Gıda. Seed'deki Easytrade finansal işlemi et-fin-8
 * (kredi kartı, Akaryakıt, 1.500,00, 6 Ekim) kullanılır. Atlas'a her koşuda
 * yeni IBAN'lı bir hesap ve Kasım MT940 ekstresi eklenir. Sonda bağ kaldırılır
 * ve hareket hesaptan çıkarılır: başka dosyanın kaydı ve sayıları değişmez.
 */
const CARD = "1500.00";

/** Ziraat (00010) banka kodlu, kontrol haneleri geçerli, her koşuda farklı TR IBAN. */
function uniqueIban(): string {
  const bban = `00010${"0"}${String(Date.now()).slice(-10)}${String(Math.floor(Math.random() * 1e6)).padStart(6, "0")}`;
  const rearranged = `${bban}292700`;
  let rem = 0;
  for (const ch of rearranged) rem = (rem * 10 + Number(ch)) % 97;
  return `TR${String(98 - rem).padStart(2, "0")}${bban}`;
}

type Matching = {
  match_status: string;
  financial_matches: { payment_method_type: string; amount: string; method: string }[];
  financial_candidates: { financial_transaction_uuid: string; payment_method_type: string; open_amount: string }[];
};

test("kart ekstresi ödemesi kredi kartı harcamasına elle bağlanır (#565)", async ({ page }) => {
  test.setTimeout(180_000);
  const api = await Api.login(DEMO.owner);
  const engagement = await engagementOf(api, DEMO.atlas.title);
  const base = `/v1/tenant/engagements/${engagement}`;
  const iban = uniqueIban();
  const box = await api.json<{ uuid: string }>(await api.post(`${base}/cash-boxes`, { iban, name: "E2E kart ödemesi" }));
  const statement = [
    ":20:E2E565KART",
    `:25:${iban}`,
    ":60F:C261101TRY10000,00",
    ":61:2611151115D1500,00NTRFNONREF//K1",
    ":86:KREDI KARTI EKSTRE ODEMESI SIRKET KARTI",
    ":62F:C261115TRY8500,00",
    "",
  ].join("\r\n");
  const up = await api.upload(`${base}/bank-statements`, {
    name: "kart-2026-11.sta",
    mimeType: "text/plain",
    buffer: Buffer.from(statement, "utf8"),
  });
  expect(up.status(), await up.text()).toBeLessThan(300);
  const tx = (
    await api.json<{ items: { uuid: string; amount: string }[] }>(
      await api.get(`${base}/bank-transactions?period=2026-11&cash_box_uuid=${box.uuid}&limit=50`),
    )
  ).items.find((x) => x.amount === CARD)!;
  expect(tx).toBeTruthy();
  const view = async () => api.json<Matching>(await api.get(`${base}/bank-transactions/${tx.uuid}/matching`));

  try {
    const before = await view();
    const card = before.financial_candidates.find((c) => c.payment_method_type === "credit_card")!;
    expect(card, JSON.stringify(before.financial_candidates)).toBeTruthy();
    expect(card.open_amount).toBe(CARD);

    await uiLogin(page, DEMO.owner);
    await page.goto(officePath(`/engagements/${engagement}/bank?kayit=bank_transaction:${tx.uuid}`));
    const panel = page.getByTestId("focused-record").getByTestId("bank-transaction-matching");
    const section = panel.getByTestId("bank-financial-candidates");
    await expect(section).toContainText("Çek, senet ya da kart ödemesine bağla");
    await expect(section).toContainText("Kredi kartı · Akaryakıt");
    await section.getByRole("checkbox", { name: "Kredi kartı · Akaryakıt seç" }).check();
    await panel.getByRole("button", { name: "Seçilenlerle eşleştir (1)" }).click();
    const linked = panel.getByTestId("bank-transaction-financial-matches");
    await expect(linked).toContainText("Kredi kartı · Akaryakıt");
    await expect(panel).toContainText("Kapattığı çek, senet ya da kart ödemesi");

    const after = await view();
    expect(after.match_status).toBe("matched");
    expect(after.financial_matches).toEqual([
      expect.objectContaining({ payment_method_type: "credit_card", amount: CARD, method: "manual" }),
    ]);
  } finally {
    await api.delete(`${base}/bank-transactions/${tx.uuid}/matches`);
    await api.put(`${base}/bank-transactions/${tx.uuid}/exclusion`, { excluded: true });
    await api.dispose();
  }
});

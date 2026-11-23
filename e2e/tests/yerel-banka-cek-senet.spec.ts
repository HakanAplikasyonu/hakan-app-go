import { expect, test } from "@playwright/test";

import { Api, DEMO, engagementOf, officePath, uiLogin } from "./lib";

/**
 * Banka hareketini çek / senede bağlama (#565): entegrasyondan (Easytrade)
 * gelen çek ve senet ödemeleri, aynı yönde ve tarihine yakın banka
 * hareketinin panelinde "Çek, senet ya da kart ödemesine bağla" bölümünde listelenir; seçilen
 * çeke bağlanır, panel "Kapattığı çek, senet ya da kart ödemesi"ni gösterir. Bağlanan çek başka
 * hareketin adayı olmaz. Gelen para (tahsilat) çek ödemesine bağlanamaz.
 *
 * Veri: demo ofisi, Atlas Gıda. Seed'deki Easytrade finansal işlemleri
 * et-fin-9 (çek, 18.600,00, 20 Ekim) ve et-fin-10 (senet, 9.400,00, 23 Ekim)
 * kullanılır. Atlas'a her koşuda yeni IBAN'lı bir hesap ve Kasım MT940
 * ekstresi eklenir. Sonda bağlar kaldırılır ve hareketler hesaptan
 * çıkarılır: başka dosyanın kaydı ve sayıları değişmez.
 */
const CHECK = "18600.00";
const NOTE = "9400.00";

/** Ziraat (00010) banka kodlu, kontrol haneleri geçerli, her koşuda farklı TR IBAN. */
function uniqueIban(): string {
  const bban = `00010${"0"}${String(Date.now()).slice(-10)}${String(Math.floor(Math.random() * 1e6)).padStart(6, "0")}`;
  const rearranged = `${bban}292700`;
  let rem = 0;
  for (const ch of rearranged) rem = (rem * 10 + Number(ch)) % 97;
  return `TR${String(98 - rem).padStart(2, "0")}${bban}`;
}

type Financial = { financial_transaction_uuid: string; payment_method_type: string; open_amount: string };
type Matching = {
  match_status: string;
  financial_matches: { payment_method_type: string; amount: string; method: string }[];
  financial_candidates: Financial[];
};

test("çek ve senet ödemesi banka hareketine elle bağlanır (#565)", async ({ page }) => {
  test.setTimeout(180_000);
  const api = await Api.login(DEMO.owner);
  const engagement = await engagementOf(api, DEMO.atlas.title);
  const base = `/v1/tenant/engagements/${engagement}`;
  const iban = uniqueIban();
  const box = await api.json<{ uuid: string }>(await api.post(`${base}/cash-boxes`, { iban, name: "E2E çek senet" }));
  const statement = [
    ":20:E2E565CEK",
    `:25:${iban}`,
    ":60F:C261101TRY50000,00",
    ":61:2611051105D18600,00NTRFNONREF//C1",
    ":86:CEK ODEMESI 0047812 ANADOLU UN",
    ":61:2611101110D9400,00NTRFNONREF//C2",
    ":86:SENET ODEMESI S-2026-118",
    ":61:2611121112C18600,00NTRFNONREF//C3",
    ":86:HAVALE GELEN E2E",
    ":62F:C261112TRY40600,00",
    "",
  ].join("\r\n");
  const up = await api.upload(`${base}/bank-statements`, {
    name: "cek-senet-2026-11.sta",
    mimeType: "text/plain",
    buffer: Buffer.from(statement, "utf8"),
  });
  expect(up.status(), await up.text()).toBeLessThan(300);
  // Yalnız bu koşunun hesabının hareketleri.
  const all = (
    await api.json<{ items: { uuid: string; amount: string; direction: "in" | "out"; cash_box_uuid: string }[] }>(
      await api.get(`${base}/bank-transactions?period=2026-11&cash_box_uuid=${box.uuid}&limit=50`),
    )
  ).items.filter((x) => x.cash_box_uuid === box.uuid);
  // Liste ucu yönü in/out verir (out = borç B, in = alacak A); debit_credit
  // yalnız eşleştirme görünümünde.
  const pick = (amount: string, dir: "in" | "out") => all.find((x) => x.amount === amount && x.direction === dir)!;
  const check = pick(CHECK, "out");
  const note = pick(NOTE, "out");
  const incoming = pick(CHECK, "in");
  expect(check && note && incoming).toBeTruthy();
  const view = async (tx: string) => api.json<Matching>(await api.get(`${base}/bank-transactions/${tx}/matching`));

  try {
    // API: çek ödemesinin adayı seed'deki çek; tahsilatın çek ödemesi adayı yok.
    const before = await view(check.uuid);
    const cheque = before.financial_candidates.find((c) => c.payment_method_type === "check")!;
    expect(cheque, JSON.stringify(before.financial_candidates)).toBeTruthy();
    expect(cheque.open_amount).toBe(CHECK);
    const inc = await view(incoming.uuid);
    expect(inc.financial_candidates.filter((c) => c.payment_method_type === "check")).toHaveLength(0);
    const wrong = await api.put(`${base}/bank-transactions/${incoming.uuid}/matches`, {
      financial_transaction_uuids: [cheque.financial_transaction_uuid],
    });
    expect(wrong.status()).toBe(422);

    // Ekran: çek ödemesinin panelinde çeki seç, bağla.
    await uiLogin(page, DEMO.owner);
    await page.goto(officePath(`/engagements/${engagement}/bank?kayit=bank_transaction:${check.uuid}`));
    const panel = page.getByTestId("focused-record").getByTestId("bank-transaction-matching");
    const section = panel.getByTestId("bank-financial-candidates");
    await expect(section).toContainText("Çek · Un alımı · çek no 0047812");
    await expect(section).toContainText("Anadolu Un Sanayi A.Ş.");
    await section.getByRole("checkbox", { name: "Çek · Un alımı · çek no 0047812 seç" }).check();
    await panel.getByRole("button", { name: "Seçilenlerle eşleştir (1)" }).click();
    const linked = panel.getByTestId("bank-transaction-financial-matches");
    await expect(linked).toContainText("Çek · Un alımı · çek no 0047812");
    await expect(linked).toContainText("Elle");
    await expect(panel).toContainText("Kapattığı çek, senet ya da kart ödemesi");

    const after = await view(check.uuid);
    expect(after.match_status).toBe("matched");
    expect(after.financial_matches).toEqual([
      expect.objectContaining({ payment_method_type: "check", amount: CHECK, method: "manual" }),
    ]);

    // Senet ödemesi senede bağlanır (API); bağlanan çek artık aday değil.
    const noteView = await view(note.uuid);
    const promissory = noteView.financial_candidates.find((c) => c.payment_method_type === "promissory_note")!;
    expect(promissory.open_amount).toBe(NOTE);
    expect(noteView.financial_candidates.map((c) => c.financial_transaction_uuid)).not.toContain(
      cheque.financial_transaction_uuid,
    );
    const linkNote = await api.put(`${base}/bank-transactions/${note.uuid}/matches`, {
      financial_transaction_uuids: [promissory.financial_transaction_uuid],
    });
    expect(linkNote.status(), await linkNote.text()).toBe(200);
    expect((await view(note.uuid)).financial_matches[0]).toMatchObject({ payment_method_type: "promissory_note", amount: NOTE });
  } finally {
    // Temizlik: bağlar kalkar, hareketler hesaptan çıkar (seed'deki çek ve
    // senet sonraki koşuda yine açık olur; Atlas'ın sayıları değişmez).
    for (const tx of [check, note, incoming]) {
      await api.delete(`${base}/bank-transactions/${tx.uuid}/matches`);
      await api.put(`${base}/bank-transactions/${tx.uuid}/exclusion`, { excluded: true });
    }
    await api.dispose();
  }
});

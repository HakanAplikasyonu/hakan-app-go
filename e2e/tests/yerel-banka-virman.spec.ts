import { expect, test } from "@playwright/test";

import { addConsentedTaxpayer, Api, DEMO, uniqueVkn } from "./lib";

/**
 * Ofis içi virman (#710): mükellefin bir banka hesabından çıkan para başka
 * banka hesabına girer. Çıkış hareketinin panel verisinde, diğer hesabın ters
 * yönlü, aynı tutarlı ve ±2 gün içindeki hareketi aday olarak gelir; tek
 * istekle bağlanır. İki hareket de "eşleşti" olur, karşı hesapları birbirinin
 * 102 hesabıdır; banka fişi çifti bir kez yazar (102.02 borç / 102.01
 * alacak). Eşleşme kaldırılınca iki taraf da açılır; fişe girmiş virman
 * kaldırılamaz.
 *
 * Veri: demo ofisine her koşuda yeni mükellef; iki TL banka hesabı (Ziraat
 * 102.01, Garanti 102.02) ve Kasım 2026 ekstreleri MT940 ile API'den
 * yüklenir: Ziraat'tan 10 Kasım'da 25.000 TL çıkış, Garanti'ye 11 Kasım'da
 * (aday) ve 20 Kasım'da (pencere dışı) 25.000 TL giriş.
 */
const run = `${Date.now()}`.slice(-6);
const taxpayer = { vkn: uniqueVkn(), title: `Virman ${run} A.Ş.` };
const PERIOD = "2026-11";

/** Banka kodlu, kontrol haneleri geçerli, her koşuda farklı TR IBAN. */
function uniqueIban(bank: string): string {
  const bban = `${bank}0${String(Date.now()).slice(-10)}${String(Math.floor(Math.random() * 1e6)).padStart(6, "0")}`;
  let rem = 0;
  for (const ch of `${bban}292700`) rem = (rem * 10 + Number(ch)) % 97;
  return `TR${String(98 - rem).padStart(2, "0")}${bban}`;
}

function mt940(
  ref: string,
  iban: string,
  lines: string[],
  closing: string,
): string {
  return [
    `:20:${ref}`,
    `:25:${iban}`,
    ":60F:C261101TRY100000,00",
    ...lines,
    closing,
    "",
  ].join("\r\n");
}

type Transfer = {
  transaction_uuid: string;
  cash_box_name: string;
  ledger_account_code: string;
} | null;
type Matching = {
  match_status: string;
  transfer: Transfer;
  transfer_candidates: {
    transaction_uuid: string;
    transaction_date: string;
    cash_box_name: string;
  }[];
  posting: {
    status: string;
    account_code: string;
    journal_draft_uuid: string | null;
  } | null;
};
type Line = { account: string; debit: string; credit: string };

test("virman: çıkış hareketi diğer bankanın girişine bağlanır, fiş 102.02 / 102.01 bir kez (#710)", async ({
  browser,
}) => {
  test.setTimeout(240_000);
  const api = await Api.login(DEMO.owner);
  const engagement = await addConsentedTaxpayer(
    browser,
    api,
    taxpayer,
    "virman",
  );
  const base = `/v1/tenant/engagements/${engagement}`;

  const ziraatIban = uniqueIban("00010");
  const garantiIban = uniqueIban("00062");
  const ziraat = await api.json<{ uuid: string }>(
    await api.post(`${base}/cash-boxes`, {
      iban: ziraatIban,
      name: "Ziraat TL",
    }),
  );
  const garanti = await api.json<{ uuid: string }>(
    await api.post(`${base}/cash-boxes`, {
      iban: garantiIban,
      name: "Garanti TL",
    }),
  );
  for (const [box, code] of [
    [ziraat.uuid, "102.01"],
    [garanti.uuid, "102.02"],
  ]) {
    const r = await api.put(`${base}/bank-ledger-accounts/${box}`, {
      account_code: code,
    });
    expect(r.status(), await r.text()).toBeLessThan(300);
  }
  for (const [name, body] of [
    [
      "ziraat-2026-11.sta",
      mt940(
        "E2E710VA",
        ziraatIban,
        [
          ":61:2611101110D25000,00NTRFNONREF//V1",
          ":86:VIRMAN GARANTI HESABINA",
        ],
        ":62F:C261110TRY75000,00",
      ),
    ],
    [
      "garanti-2026-11.sta",
      mt940(
        "E2E710VB",
        garantiIban,
        [
          ":61:2611111111C25000,00NTRFNONREF//V2",
          ":86:VIRMAN ZIRAAT HESABINDAN",
          ":61:2611201120C25000,00NTRFNONREF//V3",
          ":86:EFT GELEN MUSTERI",
        ],
        ":62F:C261120TRY150000,00",
      ),
    ],
  ]) {
    const up = await api.upload(`${base}/bank-statements`, {
      name,
      mimeType: "text/plain",
      buffer: Buffer.from(body, "utf8"),
    });
    expect(up.status(), await up.text()).toBeLessThan(300);
  }
  const txs = (
    await api.json<{
      items: {
        uuid: string;
        amount: string;
        transaction_date: string;
        debit_credit: string;
      }[];
    }>(await api.get(`${base}/bank-transactions?period=${PERIOD}&limit=20`))
  ).items;
  const out = txs.find((x) => x.transaction_date === "2026-11-10")!;
  const inNear = txs.find((x) => x.transaction_date === "2026-11-11")!;
  const inFar = txs.find((x) => x.transaction_date === "2026-11-20")!;
  expect(out && inNear && inFar).toBeTruthy();
  const tx = (id: string) => `${base}/bank-transactions/${id}`;
  const view = async (id: string) =>
    api.json<Matching>(await api.get(`${tx(id)}/matching`));

  // Öneri: yalnız ±2 gün içindeki giriş aday.
  const before = await view(out.uuid);
  expect(before.transfer).toBeNull();
  expect(before.transfer_candidates.map((c) => c.transaction_uuid)).toEqual([
    inNear.uuid,
  ]);
  expect(before.transfer_candidates[0]).toMatchObject({
    cash_box_name: "Garanti TL",
    transaction_date: "2026-11-11",
  });

  // Pencere dışındaki hareket bağlanamaz.
  expect(
    (
      await api.put(`${tx(out.uuid)}/transfer`, {
        counterpart_uuid: inFar.uuid,
      })
    ).status(),
  ).toBe(422);

  // Tek istekle eşle: iki taraf da eşleşti, karşı hesaplar birbirinin 102'si.
  const matched = await api.put(`${tx(out.uuid)}/transfer`, {
    counterpart_uuid: inNear.uuid,
  });
  expect(matched.status(), await matched.text()).toBe(200);
  const a = await view(out.uuid);
  expect(a.match_status).toBe("matched");
  expect(a.transfer).toMatchObject({
    transaction_uuid: inNear.uuid,
    cash_box_name: "Garanti TL",
    ledger_account_code: "102.02",
  });
  expect(a.posting).toMatchObject({
    status: "approved",
    account_code: "102.02",
  });
  const b = await view(inNear.uuid);
  expect(b.match_status).toBe("matched");
  expect(b.transfer).toMatchObject({
    transaction_uuid: out.uuid,
    cash_box_name: "Ziraat TL",
    ledger_account_code: "102.01",
  });
  expect(b.posting).toMatchObject({
    status: "approved",
    account_code: "102.01",
  });
  expect(b.transfer_candidates).toEqual([]);

  // Bağlı hareket ikinci kez virmana bağlanamaz.
  const again = await api.put(`${tx(inNear.uuid)}/transfer`, {
    counterpart_uuid: out.uuid,
  });
  expect(again.status()).toBe(409);
  expect(await again.text()).toContain("BANK_TRANSFER_LINKED");

  // Kaldır: iki taraf da açılır, karşı hesap kararları silinir.
  expect((await api.delete(`${tx(out.uuid)}/matches`)).status()).toBe(200);
  for (const id of [out.uuid, inNear.uuid]) {
    const v = await view(id);
    expect(v.match_status).toBe("unmatched");
    expect(v.transfer).toBeNull();
    expect(v.posting).toBeNull();
  }

  // Yeniden eşle ve fiş kes: çift bir kez yazılır, giriş aynı fişe bağlanır.
  expect(
    (
      await api.put(`${tx(out.uuid)}/transfer`, {
        counterpart_uuid: inNear.uuid,
      })
    ).status(),
  ).toBe(200);
  const made = await api.json<{ drafts: { uuid: string }[] }>(
    await api.post(`${base}/bank-postings/drafts`, { period: PERIOD }),
  );
  const lines: Line[] = [];
  for (const d of made.drafts) {
    lines.push(
      ...(
        await api.json<{ items: Line[] }>(
          await api.get(`${base}/journal-drafts/${d.uuid}`),
        )
      ).items,
    );
  }
  const transferLines = lines.filter(
    (l) =>
      l.account.startsWith("102.0") &&
      (Number(l.debit) === 25000 || Number(l.credit) === 25000),
  );
  expect(transferLines).toHaveLength(2);
  expect(Number(transferLines.find((l) => l.account === "102.02")!.debit)).toBe(
    25000,
  );
  expect(
    Number(transferLines.find((l) => l.account === "102.01")!.credit),
  ).toBe(25000);
  const da = await view(out.uuid);
  const db = await view(inNear.uuid);
  expect(da.posting?.journal_draft_uuid).toBeTruthy();
  expect(db.posting?.journal_draft_uuid).toBe(da.posting?.journal_draft_uuid);

  // Fişe girmiş virman kaldırılamaz.
  const locked = await api.delete(`${tx(out.uuid)}/matches`);
  expect(locked.status()).toBe(409);
  expect(await locked.text()).toContain("BANK_POSTING_DRAFTED");
  await api.dispose();
});

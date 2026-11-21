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
 * Ofis içi virman ekranı (#710): çıkış hareketinin panelinde "Başka banka
 * hesabıyla virman" bölümü diğer banka hesabının adayını gösterir; "Virmanı
 * eşle" ile tek tıkla bağlanır. Panel virmanı ("Garanti TL hesabına giden",
 * karşı hesap 102.02) ve "Eşleşti" durumunu gösterir; karşı hareketin
 * paneli de "Ziraat TL hesabından gelen", 102.01 gösterir. "Eşleşmeyi geri
 * al" iki tarafı açar, aday yeniden görünür.
 *
 * Veri: demo ofisine her koşuda yeni mükellef; iki TL banka hesabı (Ziraat
 * 102.01, Garanti 102.02) ve Kasım 2026 ekstreleri MT940 ile API'den
 * yüklenir: Ziraat'tan 10 Kasım'da 18.500 TL çıkış, Garanti'ye 11 Kasım'da
 * 18.500 TL giriş.
 */
const run = `${Date.now()}`.slice(-6);
const taxpayer = { vkn: uniqueVkn(), title: `Virman Ekran ${run} A.Ş.` };
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

test("virman panelden tek tıkla eşlenir, iki tarafta görünür, geri alınır (#710)", async ({
  browser,
  page,
}) => {
  test.setTimeout(240_000);
  const api = await Api.login(DEMO.owner);
  const engagement = await addConsentedTaxpayer(
    browser,
    api,
    taxpayer,
    "virmanekran",
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
        "E2E710EA",
        ziraatIban,
        [
          ":61:2611101110D18500,00NTRFNONREF//E1",
          ":86:VIRMAN GARANTI HESABINA",
        ],
        ":62F:C261110TRY81500,00",
      ),
    ],
    [
      "garanti-2026-11.sta",
      mt940(
        "E2E710EB",
        garantiIban,
        [
          ":61:2611111111C18500,00NTRFNONREF//E2",
          ":86:VIRMAN ZIRAAT HESABINDAN",
        ],
        ":62F:C261111TRY118500,00",
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
    await api.json<{ items: { uuid: string; transaction_date: string }[] }>(
      await api.get(`${base}/bank-transactions?period=${PERIOD}&limit=20`),
    )
  ).items;
  const out = txs.find((x) => x.transaction_date === "2026-11-10")!;
  const incoming = txs.find((x) => x.transaction_date === "2026-11-11")!;
  expect(out && incoming).toBeTruthy();

  await uiLogin(page, DEMO.owner);
  const open = async (id: string) => {
    await page.goto(
      officePath(
        `/engagements/${engagement}/bank?kayit=bank_transaction:${id}`,
      ),
    );
    return page
      .getByTestId("focused-record")
      .getByTestId("bank-transaction-matching");
  };

  // Aday bölümü: diğer bankanın girişi, tek tıkla eşle.
  let panel = await open(out.uuid);
  const candidates = panel.getByTestId("bank-transfer-candidates");
  await expect(candidates).toContainText("Başka banka hesabıyla virman");
  await expect(candidates).toContainText("Garanti TL hesabına giden");
  await expect(candidates).toContainText("11.11.2026");
  await candidates
    .getByRole("button", {
      name: "Garanti TL · 11.11.2026 hareketiyle virmanı eşle",
    })
    .click();
  const linked = panel.getByTestId("bank-transaction-transfer-match");
  await expect(linked).toContainText("Garanti TL hesabına giden");
  await expect(linked).toContainText("Karşı hesap 102.02");
  await expect(panel).toContainText("Eşleşti");
  await expect(panel.getByTestId("bank-transfer-candidates")).toHaveCount(0);

  // Karşı hareketin paneli de virmanı gösterir; bağlantıyla geri dönülür.
  panel = await open(incoming.uuid);
  const back = panel.getByTestId("bank-transaction-transfer-match");
  await expect(back).toContainText("Ziraat TL hesabından gelen");
  await expect(back).toContainText("Karşı hesap 102.01");
  await expect(panel).toContainText("Eşleşti");

  // Geri al: iki taraf açılır, aday yeniden görünür.
  await panel.getByRole("button", { name: "Eşleşmeyi geri al" }).click();
  await expect(
    panel.getByTestId("bank-transaction-transfer-match"),
  ).toHaveCount(0);
  await expect(panel.getByTestId("bank-transfer-candidates")).toContainText(
    "Ziraat TL hesabından gelen",
  );
  const view = await api.json<{ match_status: string; transfer: unknown }>(
    await api.get(`${base}/bank-transactions/${out.uuid}/matching`),
  );
  expect(view).toMatchObject({ match_status: "unmatched", transfer: null });
  await api.dispose();
});

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
 * Uyarılı fiş onayı (#669, #710 ofis kararı): eksik belgeli fiş engellenmez;
 * onaydan önce açıkça sorulur, gerekçe isteğe bağlı. Mükerrer olabilecek fiş
 * ENGELLENİR: API 409 JOURNAL_DUPLICATE ("yine de onayla" bayrağı da
 * geçirmez); yalnız ayrı "Yine de gönder" ve zorunlu gerekçeyle geçer.
 * Kabul edilen uyarılar ve gerekçe fişte kalır; toplu onay mükerreri atlar.
 *
 * Veri: demo ofisine her koşuda yeni mükellef; üç taslak fiş API ile:
 * eşleşmemiş banka hareketinden gelen fiş (eksik belge) ve aynı faturayı
 * kaynak gösteren iki fiş (mükerrer olabilir).
 */
const run = `${Date.now()}`.slice(-6);
const taxpayer = { vkn: uniqueVkn(), title: `Uyarı Fiş Ticaret ${run} A.Ş.` };

function declarationPeriod(now = new Date()) {
  const d = new Date(Date.UTC(now.getUTCFullYear(), now.getUTCMonth() - 1, 15));
  return { period: d.toISOString().slice(0, 7), date: d.toISOString() };
}

test("uyarılı fiş: eksik belge sorulur; mükerrer engellenir, gerekçeyle yine de gönderilir (#710)", async ({
  browser,
  page,
}) => {
  test.setTimeout(240_000);
  const api = await Api.login(DEMO.owner);
  const engagement = await addConsentedTaxpayer(
    browser,
    api,
    taxpayer,
    "uyarili",
  );
  const base = `/v1/tenant/engagements/${engagement}`;
  const { date, period } = declarationPeriod();

  const make = async (
    description: string,
    source: { source_kind: string; source_id: string },
  ) => {
    const res = await api.post(`${base}/journal-drafts`, {
      entry_date: date,
      description,
      lines: [
        { account: "770.01", debit: "100.00", credit: "0", ...source },
        { account: "102.01", debit: "0", credit: "100.00", ...source },
      ],
    });
    expect(res.status(), await res.text()).toBeLessThan(300);
    return (await api.json<{ uuid: string }>(res)).uuid;
  };
  const bank = await make(`E2E banka masrafı ${run}`, {
    source_kind: "bank_transaction",
    source_id: `9${run}`,
  });
  const dupA = await make(`E2E fatura A ${run}`, {
    source_kind: "invoice",
    source_id: `e2e-${run}`,
  });
  await make(`E2E fatura B ${run}`, {
    source_kind: "invoice",
    source_id: `e2e-${run}`,
  });

  // API: mükerrer fiş engelli; açık onay bayrağı da geçirmez, gerekçesiz
  // "yine de gönder" 400.
  for (const body of [undefined, { confirm_warnings: true }]) {
    const refused = await api.post(
      `${base}/journal-drafts/${dupA}/approve`,
      body,
    );
    expect(refused.status(), await refused.text()).toBe(409);
    const err = await refused.json();
    expect(err.error.code).toBe("JOURNAL_DUPLICATE");
    expect(JSON.stringify(err.error)).toContain("possible_duplicate");
  }
  const noReason = await api.post(`${base}/journal-drafts/${dupA}/approve`, {
    send_duplicate: true,
    reason: " ",
  });
  expect(noReason.status(), await noReason.text()).toBe(400);
  // Eksik belge yalnız soru: bayraksız 409 JOURNAL_NEEDS_CONFIRMATION.
  const ask = await api.post(`${base}/journal-drafts/${bank}/approve`);
  expect(ask.status(), await ask.text()).toBe(409);
  expect((await ask.json()).error.code).toBe("JOURNAL_NEEDS_CONFIRMATION");

  await uiLogin(page, DEMO.owner);
  await page.goto(
    officePath(`/journals?taxpayer=${engagement}&period=${period}`),
  );
  const row = page
    .locator("tr", { hasText: `E2E banka masrafı ${run}` })
    .first();
  await expect(row).toContainText("Eksik belge");
  await expect(
    page.locator("tr", { hasText: `E2E fatura A ${run}` }).first(),
  ).toContainText("Mükerrer olabilir");

  // Tekli onay: açık soru, gerekçe isteğe bağlı; vazgeçince onaylanmaz.
  await row.getByRole("button", { name: /^Onayla/ }).click();
  const dialog = page.getByRole("alertdialog").or(page.getByRole("dialog"));
  await expect(dialog).toContainText(
    "Bu fişte uyarı var. Yine de onaylansın mı?",
  );
  await expect(dialog).toContainText("Eksik belge");
  await dialog.getByRole("textbox").fill("Dekont müşteriden istendi");
  await dialog.getByRole("button", { name: "Yine de onayla" }).click();
  await expect(row).toContainText("Onaylı");
  const detail = await api.json<{
    status: string;
    approval_warnings?: string[];
    approval_note?: string;
  }>(await api.get(`${base}/journal-drafts/${bank}`));
  expect(detail).toMatchObject({
    status: "approved",
    approval_warnings: ["missing_document"],
    approval_note: "Dekont müşteriden istendi",
  });

  // Mükerrer fiş: ayrı "Yine de gönder", gerekçe zorunlu.
  const dupRow = page.locator("tr", { hasText: `E2E fatura A ${run}` }).first();
  await dupRow.getByRole("button", { name: /^Onayla/ }).click();
  await expect(dialog).toContainText("Bu fiş mükerrer olabilir; onaylanmadı.");
  await expect(dialog).toContainText("nedenini yazın");
  const send = dialog.getByRole("button", { name: "Yine de gönder" });
  await expect(send).toBeDisabled();
  await dialog.getByRole("textbox").fill("İade sonrası yeniden kesildi");
  await send.click();
  await expect(dupRow).toContainText("Onaylı");
  const after = await api.json<{
    status: string;
    approval_warnings?: string[];
    approval_note?: string;
  }>(await api.get(`${base}/journal-drafts/${dupA}`));
  expect(after).toMatchObject({
    status: "approved",
    approval_warnings: ["possible_duplicate"],
    approval_note: "İade sonrası yeniden kesildi",
  });

  // Toplu onay mükerreri atlar: kalan tek taslak (fatura B) mükerrer.
  await expect(
    page.getByRole("button", { name: /^Onaylanabilir \d+ taslağı onayla/ }),
  ).toHaveCount(0);
  await api.dispose();
});

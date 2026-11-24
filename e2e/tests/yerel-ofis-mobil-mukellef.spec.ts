import { expect, test } from "@playwright/test";

import { Api, DEMO, engagementOf } from "./lib";

/**
 * Ofis mobil uygulaması: mükellef ayrıntısı ve onay ekranının kullandığı
 * uçlar (Oğuz'un canlı geri bildirimi). Yalnız okuma; demo verisi değişmez.
 *
 * - Öneri bağlamı fiş alanı önerisinde masraf taslağını da döner
 *   (`context.expense`): onay ekranı önerilen hesap kodunu buradan okur.
 * - Kayıt sekmeleri: fatura listesi yöne göre süzülür (gelen / giden).
 * - Özet: KDV tahmini oran kırılımını, eksik belge ucu kalemleri döner.
 */
test("ofis mobil: fiş önerisinin masraf taslağı, yöne göre fatura listesi, KDV ve eksik belge özeti", async () => {
  const api = await Api.login(DEMO.owner);
  const eng = await engagementOf(api, DEMO.atlas.title);
  const base = `/v1/tenant/engagements/${eng}`;

  // Fiş alanı önerisi → bağlamda masraf taslağı → masrafın hesap kodu.
  const sugs = await api.json<{ items: { uuid: string; target_type: string }[] }>(
    await api.get(`${base}/suggestions?status=all&kind=receipt_field&limit=100`),
  );
  const onExpense = sugs.items.find((s) => s.target_type === "expense");
  expect(onExpense, "demo verisinde masraf taslağına bağlı fiş önerisi var").toBeTruthy();
  const ctx = await api.json<{ expense: { uuid: string } | null; invoice: unknown }>(
    await api.get(`${base}/suggestions/${onExpense!.uuid}/context`),
  );
  expect(ctx.expense?.uuid).toBeTruthy();
  expect(ctx.invoice).toBeNull();
  const expense = await api.json<{ uuid: string; account_code: string | null }>(
    await api.get(`${base}/expenses/${ctx.expense!.uuid}`),
  );
  expect(expense.uuid).toBe(ctx.expense!.uuid);
  expect("account_code" in expense).toBe(true);

  // Gelen / giden sekmeleri: liste yöne göre süzülür.
  for (const direction of ["incoming", "outgoing"] as const) {
    const page = await api.json<{ items: { direction: string }[] }>(
      await api.get(`${base}/invoices?direction=${direction}&limit=30&offset=0`),
    );
    for (const inv of page.items) expect(inv.direction).toBe(direction);
  }

  // Özet kartları: KDV tahmini (oran kırılımı, ödenecek / devreden) ve eksik belgeler.
  const vat = await api.json<{ rates: unknown[]; totals: { payable: string; carry_out: string } }>(
    await api.get(`${base}/vat-forecast`),
  );
  expect(Array.isArray(vat.rates)).toBe(true);
  expect(typeof vat.totals.payable).toBe("string");
  expect(typeof vat.totals.carry_out).toBe("string");
  const missing = await api.json<{ items: { kind: string }[] }>(await api.get(`${base}/document-tasks/missing`));
  expect(Array.isArray(missing.items)).toBe(true);
});

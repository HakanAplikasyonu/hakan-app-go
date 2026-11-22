import { expect, test, type Page } from "@playwright/test";

import { Api, DEMO, officePath, uiLogin } from "./lib";

/**
 * Sıradaki iş ve kapanış adımları (Oğuz'un demo geri bildirimi):
 *
 *  - "Eksik belgelere bak" o mükellefe süzülü belge toplamayı açar; başka
 *    mükellef listelenmez, fatura listesine gidilmez.
 *  - Adımın adı durumuyla çelişmez: bitmemiş adım "Fişler onaylı" / "Toplandı"
 *    demez; işletme defterinde banka, fiş ve aktarım adımı yoktur (banka ucu
 *    bunları LEDGER_NOT_BALANCE_SHEET ile reddediyordu).
 *  - Açık belge isteğine tıklayınca isteğin kendisi açılır (mükellef
 *    özetinde vurgulu satır).
 *
 * Veri: demo ofisi. Selin Yılmaz'a başka testlerin kullanmadığı bir dönemde
 * iki belge isteği açılır, sonda iptal edilir. Hüseyin Kaya işletme defteri
 * tutar (demo-seed closechain, Ekim 2026).
 */
type Step = {
  key: string;
  status: string;
  count: number;
  action: string | null;
  target_uuid?: string;
};
type Row = {
  engagement_uuid: string;
  taxpayer_title: string;
  ledger_type: string;
  next_step: string | null;
  steps: Step[];
};
type Task = { uuid: string; title: string };

const PERIOD = "2025-03";
const LEDGER_ACTIONS = ["bank_suggest", "bank_approve", "bank_drafts", "journals_approve", "export_transfer"];

async function rowOf(api: Api, title: string, period: string): Promise<Row> {
  const res = await api.get(
    `/v1/tenant/copilot/workflow?period=${period}&q=${encodeURIComponent(title)}&limit=50`,
  );
  expect(res.status(), await res.text()).toBe(200);
  const items = ((await res.json()) as { data: { items: Row[] } }).data.items;
  const row = items.find((r) => r.taxpayer_title.startsWith(title));
  expect(row, title).toBeTruthy();
  return row!;
}

/** Üst çubuğun dönemi: adresle gelen dönem global seçiciye işlenir. */
async function pinPeriod(page: Page, period: string) {
  await page.goto(officePath(`/document-collection?period=${period}`));
  await expect(page.getByTestId("document-collection-page")).toBeVisible();
}

async function copilotRow(page: Page, title: string) {
  await page.goto(officePath("/copilot"));
  await expect(page.getByRole("heading", { name: "Sıradaki iş" })).toBeVisible();
  await page.getByRole("searchbox", { name: "Mükellef ya da VKN/TCKN ara" }).fill(title);
  const row = page.getByRole("row").filter({ hasText: title });
  await expect(row.getByRole("list", { name: "Dönem kapanış adımları" })).toBeVisible();
  return row;
}

test("Sıradaki iş: süzülü eksik belgeler, durumla uyumlu adım adı, isteğin kendisi (Oğuz geri bildirimi)", async ({
  page,
}) => {
  const api = await Api.login(DEMO.owner);
  const run = Date.now();
  const selin = await rowOf(api, DEMO.selin.title, PERIOD);
  const base = `/v1/tenant/engagements/${selin.engagement_uuid}/document-tasks`;
  const created: Task[] = [];
  try {
    // İki istek: son günü yakın olan adımın hedefi.
    for (const [title, due] of [
      [`Kira sözleşmesi e2e-${run}`, "2025-04-20"],
      [`Mart ekstresi e2e-${run}`, "2025-04-10"],
    ]) {
      const res = await api.post(base, { title, due_date: due, period: PERIOD });
      expect(res.status(), await res.text()).toBe(201);
      created.push((await res.json()).data as Task);
    }
    const oldest = created[1];

    // API: belgeler adımı en eski isteği adlandırır; satır defter türünü söyler.
    const row = await rowOf(api, DEMO.selin.title, PERIOD);
    expect(row.ledger_type).toBe("bilanco");
    const docs = row.steps.find((s) => s.key === "documents")!;
    expect(docs).toMatchObject({ status: "todo", action: "open_documents", target_uuid: oldest.uuid });
    expect(docs.count).toBeGreaterThanOrEqual(2);
    expect(row.next_step).toBe("documents");

    // İşletme defteri: banka, fiş ve aktarım işi önerilmez.
    const kaya = await rowOf(api, "Hüseyin Kaya", "2026-10");
    expect(kaya.ledger_type).toBe("isletme");
    for (const s of kaya.steps) {
      expect(LEDGER_ACTIONS, `${s.key}: ${s.action}`).not.toContain(s.action ?? "");
    }
    expect(kaya.steps.find((s) => s.key === "bank")!.status).toBe("na");
    expect(kaya.steps.find((s) => s.key === "transfer")!.status).toBe("na");
    // Tek girişli defterde fiş adımı hiçbir zaman "onaylı" olmaz: ya onay
    // bekleyen masraf (todo) ya da adım yok (na).
    expect(["todo", "na"]).toContain(kaya.steps.find((s) => s.key === "journals")!.status);

    await page.setViewportSize({ width: 1440, height: 900 });
    await uiLogin(page, DEMO.owner);

    // Adım adı durumla uyumlu (Kaya, Ekim): bitmemişse "Fiş onayı".
    await pinPeriod(page, "2026-10");
    const kayaRow = await copilotRow(page, "Hüseyin Kaya");
    const kayaJournalsChip = kayaRow.locator('[data-step="journals"]');
    await expect(kayaJournalsChip).toContainText("Fiş onayı");
    await expect(kayaJournalsChip).not.toContainText("Fişler onaylı");
    await expect(kayaRow.locator('[data-step="bank"]')).toContainText("İşletme defterinde bu adım yok");
    await expect(kayaRow).not.toContainText("Banka hareketlerine hesap öner");

    // "Eksik belgelere bak" → yalnız Selin'e süzülü belge toplama.
    await pinPeriod(page, PERIOD);
    let selinRow = await copilotRow(page, DEMO.selin.title);
    await expect(selinRow).toContainText("Belge toplama");
    await expect(selinRow).not.toContainText("Toplandı");
    await selinRow.getByRole("button", { name: "Eksik belgelere bak" }).click();
    await expect(page).toHaveURL(new RegExp(`/document-collection\\?engagement=${selin.engagement_uuid}`));
    await expect(page).not.toHaveURL(/\/invoices/);
    const only = page.getByTestId("collection-only");
    await expect(only).toContainText(DEMO.selin.title);
    const rows = page.getByTestId("collection-row");
    const n = await rows.count();
    expect(n).toBeLessThanOrEqual(1);
    if (n === 1) await expect(rows.first()).toContainText(DEMO.selin.title);
    const requests = page.getByTestId("collection-open-requests");
    await expect(requests).toContainText(created[0].title);
    await expect(requests).toContainText(oldest.title);

    // Listeden bir isteği aç: mükellef özetinde o istek vurgulu.
    await requests
      .getByRole("listitem")
      .filter({ hasText: created[0].title })
      .getByTestId("collection-open-request")
      .click();
    await expect(page).toHaveURL(new RegExp(`istek=${created[0].uuid}`));
    const picked = page.locator('[data-testid="doc-task-row"][data-selected="true"]');
    await expect(picked).toHaveCount(1);
    await expect(picked).toContainText(created[0].title);

    // Süzgeç kalkınca bütün mükellefler.
    await page.goBack();
    await expect(page.getByTestId("collection-only")).toBeVisible();
    await page.getByTestId("collection-only-clear").click();
    await expect(page).not.toHaveURL(/engagement=/);
    await expect(page.getByTestId("collection-only")).toHaveCount(0);

    // Zincirdeki belge rozeti en eski isteğin kendisine götürür.
    selinRow = await copilotRow(page, DEMO.selin.title);
    await selinRow.locator('a[data-step="documents"]').click();
    await expect(page).toHaveURL(new RegExp(`/engagements/${selin.engagement_uuid}\\?istek=${oldest.uuid}`));
    await expect(picked).toHaveCount(1);
    await expect(picked).toContainText(oldest.title);
  } finally {
    for (const t of created) {
      await api.post(`${base}/${t.uuid}/cancel`, { reason: "e2e sıradaki iş" });
    }
    await api.dispose();
  }
});

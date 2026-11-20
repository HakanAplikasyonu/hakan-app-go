import { expect, test } from "@playwright/test";

import { addConsentedTaxpayerWithUser, Api, DEMO, officePath, uiLogin, uniqueVkn } from "./lib";

/**
 * Gelen belge yetersizse ofis belgeyi ekrandan yeniden ister (#571): istek
 * yeniden açılır, neyin eksik olduğu mükellefe not olarak gider, mükellef
 * yeni belgeyi aynı isteğe yükler.
 *
 * Veri: demo ofisine her koşuda yeni bir mükellef ve onun kullanıcısı; talep
 * ofis API'siyle, yüklemeler mükellef API'siyle. Başka dosyanın kaydına
 * dokunmaz.
 */
const run = `${Date.now()}`.slice(-6);
const taxpayer = { vkn: uniqueVkn(), title: `Yeniden Talep ${run} A.Ş.` };
const title = `Vergi levhasi ${run}`;
const missing = "Okunmuyor; tum sayfa gorunecek sekilde yeniden cekin";
// 1x1 saydam PNG; sonuna eklenen baytlar içeriği (ve özetini) farklı kılar.
const PNG = Buffer.from(
  "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mNkYPhfDwAChwGA60e6kgAAAABJRU5ErkJggg==",
  "base64",
);
const png = (tag: string) => Buffer.concat([PNG, Buffer.from(tag)]);

type Task = {
  uuid: string;
  status: string;
  note: string;
  request_count?: number;
  uploads?: { uuid: string }[];
};

function declarationPeriod(now = new Date()) {
  return new Date(Date.UTC(now.getUTCFullYear(), now.getUTCMonth() - 1, 15)).toISOString().slice(0, 7);
}

test("gelen belge yetersizse ofis yeniden ister; istek yeniden açılır, mükellef yenisini yükler (#571)", async ({ browser, page }) => {
  test.setTimeout(240_000);
  const api = await Api.login(DEMO.owner);
  const who = await addConsentedTaxpayerWithUser(browser, api, taxpayer, "yeniden");
  const base = `/v1/tenant/engagements/${who.engagement}/document-tasks`;
  const asTaxpayer = { "X-Taxpayer": who.taxpayerUuid };

  const created = await api.post(base, { title, note: "Guncel vergi levhasi", due_days: 3, period: declarationPeriod() });
  expect(created.status(), await created.text()).toBe(201);
  const task = (await created.json()).data as Task;

  const member = await Api.login(who.email, who.password);
  const send = async (name: string, tag: string) => {
    const res = await member.uploadFieldWith(
      `/v1/taxpayer/document-tasks/${task.uuid}/uploads`,
      "file",
      { name, mimeType: "image/png", buffer: png(tag) },
      asTaxpayer,
    );
    expect(res.status(), await res.text()).toBeLessThan(300);
  };
  await send("levha-bulanik.png", `a-${run}`);
  expect((await api.json<Task>(await api.get(`${base}/${task.uuid}`))).status).toBe("submitted");

  // Ofis: gelen belge yetersiz, "Yeniden iste" ile neyin eksik olduğu yazılır.
  await uiLogin(page, DEMO.owner);
  await page.goto(officePath(`/engagements/${who.engagement}`));
  const row = page.getByRole("listitem").filter({ hasText: title });
  await expect(row).toContainText("Geldi, kontrol et");
  await row.getByTestId("doc-task-submission").getByRole("button", { name: "Yeniden iste" }).click();
  const dialog = page.getByRole("alertdialog").or(page.getByRole("dialog"));
  await expect(dialog).toContainText("ne eksik ya da yanlış");
  await dialog.getByRole("textbox").fill(missing);
  await dialog.getByRole("button", { name: "Yeniden iste" }).click();

  // İstek yeniden açık: satır "Bekliyor", gelen belge düğmeleri kalkar.
  await expect(row).toContainText("Bekliyor");
  await expect(row.getByTestId("doc-task-submission")).toHaveCount(0);

  // Veritabanı: açık, not güncel, ikinci kez istendi.
  const reopened = await api.json<Task>(await api.get(`${base}/${task.uuid}`));
  expect(reopened.status).toBe("open");
  expect(reopened.note).toBe(missing);
  expect(reopened.request_count).toBe(2);

  // Mükellef notu görür ve yeni belgeyi aynı isteğe yükler.
  const mine = await member.json<Task>(await member.get(`/v1/taxpayer/document-tasks/${task.uuid}`, asTaxpayer));
  expect(mine.status).toBe("open");
  expect(mine.note).toBe(missing);
  await send("levha-net.png", `b-${run}`);
  await member.dispose();

  const again = await api.json<Task>(await api.get(`${base}/${task.uuid}`));
  expect(again.status).toBe("submitted");
  expect(again.uploads).toHaveLength(2);

  // Ofis ekranında satır yeniden "Geldi, kontrol et"; iki dosya da görünür.
  await page.reload();
  await expect(row).toContainText("Geldi, kontrol et");
  await expect(row.getByTestId("doc-task-submission").getByRole("button", { name: /levha-/ })).toHaveCount(2);
  await api.dispose();
});

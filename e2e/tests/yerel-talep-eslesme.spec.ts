import { expect, test } from "@playwright/test";

import { addConsentedTaxpayerWithUser, Api, DEMO, officePath, uiLogin, uniqueVkn } from "./lib";

/**
 * İstenen belge gelince talebe bağlanır (#571): ofis mükelleften belge ister
 * → mükellef belgeyi o talebe yükler → talep "geldi" olur, ofis gelen dosyayı
 * talep satırından açar ve isteği tamamlar.
 *
 * Veri: demo ofisine her koşuda yeni bir mükellef ve onun kullanıcısı; talep
 * ofis API'siyle, yükleme mükellef API'siyle (`/v1/taxpayer/document-tasks`).
 * Başka dosyanın kaydına dokunmaz.
 */
const run = `${Date.now()}`.slice(-6);
const taxpayer = { vkn: uniqueVkn(), title: `Talep Ticaret ${run} A.Ş.` };
const title = `Kira sozlesmesi ${run}`;
// 1x1 saydam PNG.
const PNG = Buffer.from(
  "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mNkYPhfDwAChwGA60e6kgAAAABJRU5ErkJggg==",
  "base64",
);

type Task = { uuid: string; status: string; title: string; uploads?: { uuid: string; file_name: string | null }[] };

/** Beyan dönemi (önceki ay): mükellef özeti varsayılan olarak bu dönemin isteklerini gösterir. */
function declarationPeriod(now = new Date()) {
  return new Date(Date.UTC(now.getUTCFullYear(), now.getUTCMonth() - 1, 15)).toISOString().slice(0, 7);
}

test("istenen belge yüklenince talebe bağlanır; ofis dosyayı açar ve isteği tamamlar (#571)", async ({ browser, page }) => {
  test.setTimeout(240_000);
  const api = await Api.login(DEMO.owner);
  const who = await addConsentedTaxpayerWithUser(browser, api, taxpayer, "talep");
  const base = `/v1/tenant/engagements/${who.engagement}/document-tasks`;

  // Ofis belge ister.
  const created = await api.post(base, {
    title,
    note: "Imzali kira sozlesmesinin taramasi",
    due_days: 3,
    period: declarationPeriod(),
  });
  expect(created.status(), await created.text()).toBe(201);
  const task = (await created.json()).data as Task;
  expect(task.status).toBe("open");

  // Mükellef isteği görür ve belgeyi o isteğe yükler.
  const member = await Api.login(who.email, who.password);
  const asTaxpayer = { "X-Taxpayer": who.taxpayerUuid };
  const mine = await member.json<{ items: Task[] }>(await member.get("/v1/taxpayer/document-tasks?limit=50", asTaxpayer));
  expect(mine.items.map((t) => t.uuid)).toContain(task.uuid);
  const upload = await member.uploadFieldWith(
    `/v1/taxpayer/document-tasks/${task.uuid}/uploads`,
    "file",
    { name: "kira-sozlesmesi.png", mimeType: "image/png", buffer: PNG },
    asTaxpayer,
  );
  expect(upload.status(), await upload.text()).toBeLessThan(300);
  const result = (await upload.json()).data as { task: Task; matched: boolean };
  expect(result.task.status).toBe("submitted");
  await member.dispose();

  // Veritabanı: talep "geldi", yüklenen dosya talebe bağlı.
  const arrived = await api.json<Task>(await api.get(`${base}/${task.uuid}`));
  expect(arrived.status).toBe("submitted");
  expect(arrived.uploads).toHaveLength(1);
  expect(arrived.uploads![0].file_name).toContain("kira-sozlesmesi");

  // Ofis: mükellef özetinde istek satırında "Geldi, kontrol et" ve dosya.
  await uiLogin(page, DEMO.owner);
  await page.goto(officePath(`/engagements/${who.engagement}`));
  const row = page.getByRole("listitem").filter({ hasText: title });
  await expect(row).toBeVisible();
  await expect(row).toContainText("Geldi, kontrol et");
  const submission = row.getByTestId("doc-task-submission");
  const file = submission.getByRole("button", { name: /kira-sozlesmesi/ });
  await expect(file).toBeVisible();
  const [download] = await Promise.all([page.waitForEvent("download"), file.click()]);
  expect(download.suggestedFilename()).toContain("kira-sozlesmesi");

  // Belge doğru: istek tamamlanır ve açık istekler listesinden düşer.
  await submission.getByRole("button", { name: "Belge tamam" }).click();
  await expect(page.getByRole("listitem").filter({ hasText: title })).toHaveCount(0);
  const done = await api.json<Task>(await api.get(`${base}/${task.uuid}`));
  expect(done.status).toBe("matched");
  await api.dispose();
});

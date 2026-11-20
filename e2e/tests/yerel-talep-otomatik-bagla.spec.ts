import { expect, test } from "@playwright/test";

import { addConsentedTaxpayer, Api, DEMO, officePath, uiLogin, uniqueVkn } from "./lib";

/**
 * Talebe değil başka yoldan gelen belge (#571). WhatsApp'tan "belge" olarak,
 * görev seçilmeden kaydedilen dosya:
 * - mükellefin tek açık belge isteği varsa o isteğe kendiliğinden bağlanır;
 * - açık istek yoksa ya da birden fazlaysa bağlanmaz (sistem tahmin etmez);
 *   ofis "Talebe bağla" ile isteği seçer.
 *
 * Veri: demo ofisine her koşuda yeni bir mükellef; istekler ve belgeler ofis
 * API'siyle (`…/whatsapp/captures`, uzantının kullandığı uç). Başka dosyanın
 * kaydına dokunmaz.
 */
const run = `${Date.now()}`.slice(-6);
const taxpayer = { vkn: uniqueVkn(), title: `Otomatik Talep ${run} A.Ş.` };
const PNG = Buffer.from(
  "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mNkYPhfDwAChwGA60e6kgAAAABJRU5ErkJggg==",
  "base64",
);

type Capture = { uuid: string; task_uuid: string | null; task_title: string | null; auto_linked: boolean; open_requests: number };
type Task = { uuid: string; status: string; uploads?: { uuid: string }[] };

test("görevsiz gelen belge tek açık isteğe kendiliğinden bağlanır; birden fazlaysa ofis seçer (#571)", async ({ browser, page }) => {
  test.setTimeout(240_000);
  const api = await Api.login(DEMO.owner);
  const engagement = await addConsentedTaxpayer(browser, api, taxpayer, "otomatik");
  const base = `/v1/tenant/engagements/${engagement}`;

  const save = async (name: string) => {
    const res = await api.postForm(`${base}/whatsapp/captures`, {
      target: "document",
      file: { name, mimeType: "image/png", buffer: Buffer.concat([PNG, Buffer.from(`${name}-${run}`)]) },
    });
    expect(res.status(), await res.text()).toBe(201);
    return (await res.json()).data as Capture;
  };
  const request = async (title: string) => {
    const res = await api.post(`${base}/document-tasks`, { title, due_days: 3 });
    expect(res.status(), await res.text()).toBe(201);
    return (await res.json()).data as Task;
  };
  const task = async (id: string) => api.json<Task>(await api.get(`${base}/document-tasks/${id}`));

  // Açık istek yok: belge saklanır, hiçbir isteğe bağlanmaz.
  const loose = await save(`belge-a-${run}.png`);
  expect(loose).toMatchObject({ task_uuid: null, auto_linked: false, open_requests: 0 });

  // Tek açık istek: gelen belge o isteğin karşılığıdır.
  const only = await request(`Kira sozlesmesi ${run}`);
  const linked = await save(`belge-b-${run}.png`);
  expect(linked).toMatchObject({ task_uuid: only.uuid, auto_linked: true, open_requests: 1 });
  const arrived = await task(only.uuid);
  expect(arrived.status).toBe("submitted");
  expect(arrived.uploads).toHaveLength(1);

  // İki açık istek: sistem tahmin etmez, belge görevsiz kalır.
  const second = await request(`Vergi levhasi ${run}`);
  const third = await request(`Imza sirkuleri ${run}`);
  const unsure = await save(`belge-c-${run}.png`);
  expect(unsure).toMatchObject({ task_uuid: null, auto_linked: false, open_requests: 2 });
  expect((await task(second.uuid)).status).toBe("open");
  expect((await task(third.uuid)).status).toBe("open");

  // Ofis: WhatsApp kayıtlarında belge satırında "Talebe bağla", istek seçilir.
  await uiLogin(page, DEMO.owner);
  await page.goto(officePath("/whatsapp"));
  const row = page.getByRole("row").filter({ hasText: `belge-c-${run}.png` });
  await expect(row).toBeVisible();
  await row.getByRole("button", { name: "Talebe bağla" }).click();
  const dialog = page.getByRole("dialog");
  await expect(dialog).toContainText(`Vergi levhasi ${run}`);
  await expect(dialog).toContainText(`Imza sirkuleri ${run}`);
  // Karşılığı gelmiş istek aday değildir.
  await expect(dialog).not.toContainText(`Kira sozlesmesi ${run}`);
  await dialog.getByRole("button", { name: `“Imza sirkuleri ${run}” isteğine bağla` }).click();
  await expect(row).toContainText(`“Imza sirkuleri ${run}” talebine eklendi`);
  await expect(row.getByRole("button", { name: "Talebe bağla" })).toHaveCount(0);

  // Veritabanı: seçilen istek "geldi", öbürü hâlâ açık.
  const chosen = await task(third.uuid);
  expect(chosen.status).toBe("submitted");
  expect(chosen.uploads).toHaveLength(1);
  expect((await task(second.uuid)).status).toBe("open");

  // Bağlanmış belge ikinci kez bağlanamaz.
  const again = await api.put(`${base}/whatsapp/captures/${unsure.uuid}/task`, { task_uuid: second.uuid });
  expect(again.status()).toBe(409);
  expect(await again.text()).toContain("CAPTURE_NOT_LINKABLE");
  expect((await task(second.uuid)).status).toBe("open");
  await api.dispose();
});

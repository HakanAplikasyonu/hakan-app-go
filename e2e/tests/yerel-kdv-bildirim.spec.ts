import { expect, test } from "@playwright/test";

import {
  addConsentedTaxpayerWithUser,
  Api,
  DEMO,
  expectNoRawText,
  officePath,
  uiLogin,
  uniqueVkn,
} from "./lib";

/**
 * Mükellefe tek tıkla KDV bildirimi ve gönderim kanıtı (#780 B): ofis rapor
 * sayfasından "Mükellefe gönder" der; mükellef bildirimi kendi panelinde
 * görür; mükellef okuyunca ofis ekranında kanal bazında "Okundu" zamanı
 * çıkar.
 *
 * Veri: demo ofisine her koşuda yeni bir mükellef ve onun kullanıcısı.
 * Faturası olmadığı için bu ay "ödeme çıkmayacak". Başka dosyanın kaydına
 * dokunmaz.
 */
const run = `${Date.now()}`.slice(-6);
const taxpayer = { vkn: uniqueVkn(), title: `KDV Bildirim ${run} Ltd.` };
const note = `Bu ay odeme cikmiyor ${run}`;

type Delivery = { channel: string; status: string; read_at: string | null };
type Notice = {
  uuid: string;
  outcome: string;
  message: string | null;
  deliveries: Delivery[];
};
type Inbox = {
  items: {
    uuid: string;
    channel: string;
    title: string;
    body: string;
    payload: Record<string, unknown>;
  }[];
};

test("ofis KDV durumunu mükellefe gönderir; mükellef okuyunca ofiste okundu zamanı görünür (#780)", async ({
  browser,
  page,
}) => {
  test.setTimeout(240_000);
  const api = await Api.login(DEMO.owner);
  const who = await addConsentedTaxpayerWithUser(
    browser,
    api,
    taxpayer,
    "kdvbildirim",
  );
  const base = `/v1/tenant/engagements/${who.engagement}/vat-forecast/notices`;

  // Ofis: rapor sayfasındaki kartta henüz gönderim yok; tek tıkla gönderir.
  await uiLogin(page, DEMO.owner);
  await page.goto(officePath(`/engagements/${who.engagement}/report`));
  const card = page.getByTestId("vat-notice-card");
  await expect(card).toContainText("Henüz KDV bildirimi gönderilmedi.");
  await card.getByRole("button", { name: "Mükellefe gönder" }).click();
  const dialog = page.getByRole("dialog");
  await dialog.getByRole("textbox").fill(note);
  await dialog.getByRole("button", { name: "Gönder", exact: true }).click();
  await expect(dialog).toBeHidden();

  // Kartta gönderim: ödeme çıkmayacak, not, iki kanal.
  await expect(card).toContainText("Ödeme çıkmayacak");
  await expect(card).toContainText(note);
  await expect(card).toContainText("Uygulama içi");
  await expect(card).toContainText("E-posta");
  await expectNoRawText(card);

  // Veritabanı: bildirim kaydı ve kanal başına bir gönderim.
  const list = await api.json<{ items: Notice[] }>(await api.get(base));
  expect(list.items).toHaveLength(1);
  const sent = list.items[0];
  expect(sent.outcome).toBe("no_payment");
  expect(sent.message).toBe(note);
  expect(sent.deliveries.map((d) => d.channel).sort()).toEqual([
    "email",
    "inapp",
  ]);
  expect(sent.deliveries.every((d) => d.read_at === null)).toBe(true);

  // Mükellef: bildirim kendi gelen kutusunda; okur.
  const member = await Api.login(who.email, who.password);
  const asTaxpayer = { "X-Taxpayer": who.taxpayerUuid };
  const inbox = await member.json<Inbox>(
    await member.get("/v1/taxpayer/notifications?limit=50", asTaxpayer),
  );
  const mine = inbox.items.find(
    (n) => n.channel === "inapp" && n.payload?.notice_uuid === sent.uuid,
  );
  expect(mine, JSON.stringify(inbox.items.map((n) => n.title))).toBeTruthy();
  expect(mine!.title).toContain("KDV ödemeniz çıkmıyor");
  expect(mine!.body).toContain(note);
  const read = await member.post(
    `/v1/taxpayer/notifications/${mine!.uuid}/read`,
    {},
    asTaxpayer,
  );
  expect(read.status(), await read.text()).toBeLessThan(300);
  await member.dispose();

  // Kanıt: uygulama içi gönderimin okunma zamanı kayıtlı ve ekranda.
  const after = await api.json<{ items: Notice[] }>(await api.get(base));
  expect(
    after.items[0].deliveries.find((d) => d.channel === "inapp")?.read_at,
  ).toBeTruthy();
  await page.reload();
  await expect(card).toContainText("Okundu");
  await api.dispose();
});

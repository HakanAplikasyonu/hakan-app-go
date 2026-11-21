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
 * Mükellefe tek tıkla KDV bildirimi (#780 B): ofis rapor sayfasından
 * "Mükellefe gönder" der; ne söylendiği o anki rakamlarla kaydedilir ve
 * kartta görünür; mükellef bildirimi kendi panelinde görür.
 *
 * Veri: demo ofisine her koşuda yeni bir mükellef ve onun kullanıcısı.
 * Faturası olmadığı için bu ay "ödeme çıkmayacak". Başka dosyanın kaydına
 * dokunmaz.
 */
const run = `${Date.now()}`.slice(-6);
const taxpayer = { vkn: uniqueVkn(), title: `KDV Bildirim ${run} Ltd.` };
const note = `Bu ay odeme cikmiyor ${run}`;

type Notice = {
  uuid: string;
  outcome: string;
  message: string | null;
  message_ref: string | null;
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

test("ofis KDV durumunu mükellefe gönderir; ne söylendiği kaydedilir, mükellef panelinde görür (#780)", async ({
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

  // Kartta gönderim: ödeme çıkmayacak ve not.
  await expect(card).toContainText("Ödeme çıkmayacak");
  await expect(card).toContainText(note);
  await expectNoRawText(card);

  // Veritabanı: mükellefe ne söylendiği kayıtlı; iletişim merkezi bağlantısı henüz boş.
  const list = await api.json<{ items: Notice[] }>(await api.get(base));
  expect(list.items).toHaveLength(1);
  const sent = list.items[0];
  expect(sent.outcome).toBe("no_payment");
  expect(sent.message).toBe(note);
  expect(sent.message_ref).toBeNull();

  // Mükellef: bildirim kendi gelen kutusunda.
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
  await member.dispose();

  await api.dispose();
});

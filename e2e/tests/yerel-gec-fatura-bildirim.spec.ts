import { expect, test } from "@playwright/test";

import {
  addConsentedTaxpayer,
  Api,
  DEMO,
  deriveUbl,
  officePath,
  uiLogin,
  uniqueVkn,
  waitForUploads,
} from "./lib";

/**
 * Geç gelen alış faturası mali müşavire bildirilir (#669 A.1): personel
 * alış faturasının KDV'sini sonraki aya aldığında onay gerekmez, ama
 * mükellefe atanmış müşavire uygulama içi bildirim gider: kim yaptı, hangi
 * fatura, hangi aya. Değişikliği yapan kişiye bildirim gitmez.
 *
 * Veri: demo ofisine her koşuda yeni bir mükellef; Atlas'ın gelen faturası
 * (atlas-05, Ekim 2026) bu mükellefe türetilir. Müşavir (Ayşe Demir) ve
 * stajyer (Can Kaya) mükellefe atanır. Başka dosyanın kaydına dokunmaz.
 */
const MUSAVIR = "musavir@demo.hakan.local";

type Notification = {
  title: string;
  body: string;
  action_url?: string;
  payload: Record<string, unknown>;
};

test("geç gelen alış faturası sonraki aya alınınca müşavire bildirim gider (#669)", async ({
  page,
  browser,
}) => {
  test.setTimeout(240_000);
  await page.setViewportSize({ width: 1440, height: 900 });
  const run = `${Date.now()}`.slice(-6);
  const taxpayer = { vkn: uniqueVkn(), title: `Geç Fatura ${run} A.Ş.` };
  const owner = await Api.login(DEMO.owner);
  const engagement = await addConsentedTaxpayer(
    browser,
    owner,
    taxpayer,
    "gecfatura",
  );
  const base = `/v1/tenant/engagements/${engagement}`;
  const up = await owner.upload(
    `${base}/ubl-uploads`,
    deriveUbl("atlas-05-iade-alis.xml", taxpayer.vkn, taxpayer.title),
  );
  expect(up.status(), await up.text()).toBeLessThan(300);
  await waitForUploads(owner, engagement, 1);
  const invoice = (
    await owner.json<{
      items: { uuid: string; invoice_number: string; direction: string }[];
    }>(await owner.get(`${base}/invoices?limit=5`))
  ).items[0];
  expect(invoice.direction).toBe("incoming");

  for (const email of [MUSAVIR, DEMO.intern]) {
    const members = await owner.json<{
      items: { user: { uuid: string; email: string } }[];
    }>(await owner.get(`/v1/tenant/members?q=${encodeURIComponent(email)}`));
    const member = members.items.find((m) => m.user.email === email);
    expect(member, email).toBeTruthy();
    const assigned = await owner.put(
      `${base}/assignments/${member!.user.uuid}`,
    );
    expect(assigned.status(), await assigned.text()).toBeLessThan(300);
  }
  await owner.dispose();

  const musavir = await Api.login(MUSAVIR);
  const notices = async () =>
    (
      await musavir.json<{ items: Notification[] }>(
        await musavir.get("/v1/notifications?limit=50&channel=inapp"),
      )
    ).items.filter((n) => n.payload?.invoice_uuid === invoice.uuid);
  expect(await notices()).toHaveLength(0);

  // Stajyer faturanın KDV'sini Kasım 2026 beyanına alır.
  await uiLogin(page, DEMO.intern);
  await page.goto(
    officePath(`/engagements/${engagement}/documents?belge=${invoice.uuid}`),
  );
  const box = page.getByRole("region", { name: "KDV indirim dönemi" });
  await box.getByRole("button", { name: "İndirim dönemini değiştir" }).click();
  await box.getByLabel("KDV indirim ayı").fill("2026-11");
  await box.getByRole("button", { name: "Kaydet" }).click();
  await expect(box.getByTestId("vat-deduction-note")).toContainText(
    "Kasım 2026",
  );

  // Müşavire tek bildirim: kim, hangi fatura, hangi ay.
  await expect.poll(async () => (await notices()).length).toBe(1);
  const [notice] = await notices();
  expect(notice.title).toBe("Geç gelen alış faturası işlendi");
  expect(notice.body).toContain("Can Kaya");
  expect(notice.body).toContain(taxpayer.title);
  expect(notice.body).toContain(invoice.invoice_number);
  expect(notice.body).toContain("Kasım 2026 KDV beyanına aldı");
  expect(notice.payload.vat_period).toBe("2026-11");
  expect(notice.payload.document_period).toBe("2026-10");
  expect(notice.action_url).toContain(`belge=${invoice.uuid}`);
  await musavir.dispose();

  // Değişikliği yapan stajyere bildirim gitmez.
  const intern = await Api.login(DEMO.intern);
  const own = (
    await intern.json<{ items: Notification[] }>(
      await intern.get("/v1/notifications?limit=50&channel=inapp"),
    )
  ).items.filter((n) => n.payload?.invoice_uuid === invoice.uuid);
  expect(own).toHaveLength(0);
  await intern.dispose();

  // Müşavir bildirimi üst çubuktaki zilde görür.
  const context = await browser.newContext();
  const mpage = await context.newPage();
  await mpage.setViewportSize({ width: 1440, height: 900 });
  await uiLogin(mpage, MUSAVIR);
  await mpage.goto(officePath());
  await mpage
    .getByRole("button", { name: /Bildirimler|okunmamış bildirim/ })
    .first()
    .click();
  await expect(
    mpage.getByText("Geç gelen alış faturası işlendi").first(),
  ).toBeVisible();
  await context.close();
});

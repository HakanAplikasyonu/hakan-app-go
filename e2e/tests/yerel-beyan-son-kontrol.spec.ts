import { expect, test } from "@playwright/test";

import {
  addConsentedTaxpayer,
  Api,
  DEMO,
  mailToken,
  officePath,
  registerWithToken,
  uniqueVkn,
} from "./lib";

/**
 * Beyan son kontrolü (#669, ofis kararı): ofiste herkes beyanı hazırlar;
 * "kontrol edildi" ve "verildi" işaretini yalnız sorumlu (sahip, yönetici)
 * koyar. Personel hazırlar ama işaretleyemez (sunucu 403, ekranda düğme
 * pasif ve nedeni yazılı); sahip işaretler; kim / ne zaman görünür.
 *
 * Ofis ayarı (#710): son kontrol varsayılan açık; ofis kapatınca personel de
 * "kontrol edildi" işaretler (ekranda düğme etkin). Ayar test sonunda yine
 * açılır (ayar ofis genelidir; ona bağlı tek test bu dosyadır).
 *
 * Veri: demo ofisine her koşuda yeni mükellef ve personel rolüyle davet
 * edilen yeni bir kullanıcı; beyan o mükellefin önceki ay KDV1'i.
 */
const run = `${Date.now()}`.slice(-7);
const taxpayer = { vkn: uniqueVkn(), title: `Son Kontrol Ticaret ${run} A.Ş.` };
const memberEmail = `personel-${run}@example.com`;
const password = "Personel-e2e-123!";
const ONLY = "Kontrol edildi ve verildi işaretini yalnız sorumlu (ofis sahibi ya da yönetici) koyar";

function declarationPeriod(now = new Date()) {
  const d = new Date(Date.UTC(now.getUTCFullYear(), now.getUTCMonth() - 1, 15));
  return { period: d.toISOString().slice(0, 7), lastDay: new Date(Date.UTC(d.getUTCFullYear(), d.getUTCMonth() + 1, 0)) };
}

type Preview = {
  status: string;
  open_warnings: string[];
  record: { checked_at: string | null; checked_by_name?: string; filed_by_name?: string; prepared_by_name?: string } | null;
};

test("beyan: personel hazırlar, yalnız sorumlu işaretler; ofis ayarı kapatınca personel de işaretler (#710)", async ({ browser, page }) => {
  test.setTimeout(240_000);
  const owner = await Api.login(DEMO.owner);
  const engagement = await addConsentedTaxpayer(browser, owner, taxpayer, "sonkontrol");
  const { period, lastDay } = declarationPeriod();
  const url = `/v1/tenant/engagements/${engagement}/tax-returns/kdv1/${period}`;

  // Personel rolüyle davet, kayıt, mükellefe atama.
  const invited = await owner.post("/v1/tenant/invitations", { email: memberEmail, role_key: "member" });
  expect(invited.status(), await invited.text()).toBeLessThan(300);
  const token = await mailToken(memberEmail, "/invite/");
  await registerWithToken(page, token, `/invite/${token}`, { name: "Ece", surname: "Personel", password });
  await page.getByRole("button", { name: "Daveti kabul et" }).click();
  await page.waitForURL((u) => u.pathname.startsWith(officePath()));
  const members = await owner.json<{ items: { user: { uuid: string; email: string } }[] }>(
    await owner.get(`/v1/tenant/members?q=${encodeURIComponent(memberEmail)}`),
  );
  const member = members.items.find((m) => m.user.email === memberEmail);
  expect(member, "davet edilen personel").toBeTruthy();
  const assigned = await owner.put(`/v1/tenant/engagements/${engagement}/assignments/${member!.user.uuid}`);
  expect(assigned.status(), await assigned.text()).toBeLessThan(300);

  // Personel beyanı hazırlar (uyarıları onaylayarak).
  const staff = await Api.login(memberEmail, password);
  const draft = await staff.json<Preview>(await staff.get(url));
  const prepared = await staff.post(`${url}/prepare`, { acknowledged: draft.open_warnings });
  expect(prepared.status(), await prepared.text()).toBe(200);

  // Sunucu: personel kontrol edildi / verildi işaretleyemez.
  expect((await staff.post(`${url}/check`)).status()).toBe(403);
  const filedOn = lastDay.toISOString().slice(0, 10);
  expect((await staff.post(`${url}/file`, { filed_on: filedOn })).status()).toBe(403);

  // Ekran: düğmeler pasif, nedeni yazılı; hazırlayan görünür.
  await page.goto(officePath(`/engagements/${engagement}/tax-returns`));
  const status = page.getByRole("region", { name: "Beyan durumu" });
  await expect(status).toContainText("Hazırlandı");
  await expect(status.getByRole("button", { name: "Kontrol edildi olarak işaretle" })).toBeDisabled();
  await expect(status.getByRole("button", { name: "Verildi olarak işaretle" })).toBeDisabled();
  await expect(status).toContainText(ONLY);
  await expect(status.getByTestId("tax-return-trail")).toContainText("Hazırlayan: Ece Personel");

  // Ofis ayarı (#710): varsayılan açık; kapatılınca personel de işaretler.
  const settings = "/v1/tenant/tax-return-settings";
  const current = await owner.json<{ final_check_required: boolean }>(await owner.get(settings));
  expect(current.final_check_required).toBe(true);
  expect((await staff.put(settings, { final_check_required: false })).status()).toBe(403);
  try {
    const off = await owner.put(settings, { final_check_required: false });
    expect(off.status(), await off.text()).toBe(200);
    await page.reload();
    await expect(status).not.toContainText(ONLY);
    await status.getByRole("button", { name: "Kontrol edildi olarak işaretle" }).click();
    await expect
      .poll(async () => (await staff.json<Preview>(await staff.get(url))).record?.checked_by_name)
      .toBe("Ece Personel");
  } finally {
    const on = await owner.put(settings, { final_check_required: true });
    expect(on.status(), await on.text()).toBe(200);
  }
  // Ayar yeniden açık: personel yine işaretleyemez.
  expect((await staff.post(`${url}/file`, { filed_on: filedOn })).status()).toBe(403);

  // Sorumlu (sahip) kontrol edildi ve verildi işaretler; kim / ne zaman kaydedilir.
  const checked = await owner.json<Preview>(await owner.post(`${url}/check`));
  expect(checked.record?.checked_at).toBeTruthy();
  expect(checked.record?.checked_by_name).toBeTruthy();
  const filed = await owner.post(`${url}/file`, { filed_on: filedOn });
  expect(filed.status(), await filed.text()).toBe(200);

  await page.reload();
  await expect(status).toContainText("Verildi");
  const trail = status.getByTestId("tax-return-trail");
  await expect(trail).toContainText("Kontrol eden:");
  await expect(trail).toContainText("Veren:");
  await owner.dispose();
  await staff.dispose();
});

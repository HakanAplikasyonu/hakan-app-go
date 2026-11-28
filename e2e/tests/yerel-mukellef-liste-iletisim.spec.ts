import { expect, test } from "@playwright/test";

import { Api, DEMO, engagementOf, officePath, uiLogin } from "./lib";

/**
 * Mükellef listesinde iletişim (#780, Oğuz: "telefon numarasını
 * göremiyorum"): Mükellefler tablosunda telefon ve e-posta sütunu. Kayıtlı
 * değilse "Kayıtlı değil" yazar.
 *
 * Veri: demo ofisi, Atlas Gıda; telefon ve e-posta test boyunca değişir,
 * sonunda eski değerine döner.
 */
type Contact = { phone: string | null; email: string | null };

test("mükellef listesi telefon ve e-postayı gösterir (#780)", async ({ page }) => {
  const office = await Api.login(DEMO.owner);
  const atlas = await engagementOf(office, DEMO.atlas.title);
  const path = `/v1/tenant/engagements/${atlas}/contacts`;
  const before = await office.json<Contact>(await office.get(path));
  try {
    expect((await office.put(path, { phone: "+90 532 987 65 43", email: "muhasebe@atlas.example" })).ok()).toBeTruthy();

    // API: dönem matrisi satırı iletişimi taşır (sunucu biçimiyle).
    const m = await office.json<{ items: { engagement_uuid: string; taxpayer_phone: string | null; taxpayer_email: string | null }[] }>(
      await office.get(`/v1/tenant/periods/matrix?periods=1&limit=200`),
    );
    const row = m.items.find((r) => r.engagement_uuid === atlas);
    expect(row).toMatchObject({ taxpayer_phone: "0532 987 65 43", taxpayer_email: "muhasebe@atlas.example" });

    await uiLogin(page, DEMO.owner);
    await page.goto(officePath("/taxpayers"));
    await expect(page.getByRole("columnheader", { name: "İletişim" })).toBeVisible();
    await page.getByPlaceholder("Unvan ya da VKN ara").fill(DEMO.atlas.title);
    const tr = page.getByRole("row").filter({ hasText: DEMO.atlas.title });
    await expect(tr.getByRole("link", { name: "0532 987 65 43" })).toHaveAttribute("href", "tel:05329876543");
    await expect(tr.getByRole("link", { name: "muhasebe@atlas.example" })).toHaveAttribute("href", "mailto:muhasebe@atlas.example");

    // Boşaltılınca "Kayıtlı değil".
    expect((await office.put(path, { phone: "", email: "" })).ok()).toBeTruthy();
    await page.reload();
    await page.getByPlaceholder("Unvan ya da VKN ara").fill(DEMO.atlas.title);
    await expect(page.getByRole("row").filter({ hasText: DEMO.atlas.title }).getByText("Kayıtlı değil")).toBeVisible();
  } finally {
    await office.put(path, { phone: before.phone ?? "", email: before.email ?? "" });
    await office.dispose();
  }
});

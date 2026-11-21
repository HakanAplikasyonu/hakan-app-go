import { expect, test } from "@playwright/test";

import { Api, DEMO, officePath, uiLogin } from "./lib";

/**
 * Otomasyon kuralları (#780 C, İletişim merkezi): Ayarlar > Otomasyonlar
 * demo ofisinde en az 4 aktif kuralla gelir; hazır kurallar pasif durur ve
 * satırdaki anahtarla tek tıkla açılır; "Yeni kural" penceresi açılır.
 *
 * Veri: demo ofisi. Açılan hazır kural testin sonunda API ile eski haline
 * (pasif) döner; başka bir şey kaydedilmez.
 */
type Rule = {
  uuid: string;
  name: string;
  template_code: string;
  active: boolean;
};
type Overview = {
  enabled: boolean;
  stats: { total: number; active: number; sent: number; drafts: number };
  items: Rule[];
};

test("otomasyonlar: demo kuralları, hazır kuralı tek tıkla açma (#780)", async ({
  page,
}) => {
  const api = await Api.login(DEMO.owner);
  const before = await api.json<Overview>(
    await api.get("/v1/tenant/automations"),
  );
  expect(before.stats.active).toBeGreaterThanOrEqual(4);
  const passive = before.items.find((r) => r.template_code !== "" && !r.active);
  expect(passive, "pasif hazır kural yok").toBeTruthy();

  try {
    await page.setViewportSize({ width: 1440, height: 900 });
    await uiLogin(page, DEMO.owner);
    await page.goto(officePath("/settings"));

    await expect(page.getByText("Toplam gönderim")).toBeVisible();
    await page.getByRole("tab", { name: "Aktif", exact: true }).click();
    await expect(page.getByTestId("automation-row")).toHaveCount(
      before.stats.active,
    );

    await page.getByRole("tab", { name: "Pasif", exact: true }).click();
    await page
      .getByRole("switch", { name: `${passive!.name} kuralını aç/kapat` })
      .click();
    await expect
      .poll(async () => {
        const o = await api.json<Overview>(
          await api.get("/v1/tenant/automations"),
        );
        return o.items.find((r) => r.uuid === passive!.uuid)?.active;
      })
      .toBe(true);

    await page.getByRole("button", { name: "Yeni kural" }).click();
    await expect(page.getByRole("dialog")).toContainText(
      "Yeni otomasyon kuralı",
    );
    await page.keyboard.press("Escape");
  } finally {
    await api.put(`/v1/tenant/automations/${passive!.uuid}/active`, {
      active: false,
    });
  }
});

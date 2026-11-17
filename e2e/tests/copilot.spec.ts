import { expect, test } from "@playwright/test";

import { scan } from "./a11y";
import { Api, DEMO, officePath, uiLogin } from "./lib";

/**
 * Co-pilot (#392): "Sıradaki iş" ekranı her mükellefin kapanış zincirini
 * gösterir; komut (ekrandaki kutu ya da Ctrl+K) onay bekleyen bir plana
 * dönüşür ve hiçbir adım onaysız çalışmaz. Vazgeçilen plan iptal edilir.
 */
test.describe("co-pilot", () => {
  test("sıradaki iş listesi ve komuttan onay bekleyen plan", async ({ page }) => {
    await uiLogin(page, DEMO.owner);
    await page.goto(officePath("/copilot"));
    await expect(page.getByRole("heading", { name: "Sıradaki iş" })).toBeVisible();
    const atlasRow = page.getByRole("row").filter({ hasText: DEMO.atlas.title });
    await expect(atlasRow).toBeVisible();
    await expect(atlasRow.getByRole("list", { name: "Dönem kapanış adımları" })).toBeVisible();

    // Komut paletten: "Co-pilot'a sor".
    await page.keyboard.press("Control+k");
    await page.keyboard.type("Atlas Gıda ekim fişlerini Luca'ya aktar");
    await page.getByRole("option", { name: /Co-pilot'a sor/ }).click();
    const dialog = page.getByRole("dialog", { name: "Onay bekleyen işlemler" });
    await expect(dialog).toBeVisible();
    await expect(dialog.getByText("Fişleri muhasebe programına aktar")).toBeVisible();
    await expect(dialog.getByRole("button", { name: /Onayla ve çalıştır/ })).toBeEnabled();
    await scan(page, "co-pilot onay penceresi", { include: '[role="dialog"]' });

    // Vazgeç: plan iptal, hiçbir adım çalışmadı.
    await dialog.getByRole("button", { name: "Vazgeç" }).click();
    await expect(dialog).toBeHidden();
    const api = await Api.login(DEMO.owner);
    await expect
      .poll(async () => {
        const plans = await api.json<{ items: { command: string; status: string; steps: { status: string }[] }[] }>(
          await api.get("/v1/tenant/copilot/plans"),
        );
        const plan = plans.items.find((p) => p.command.includes("Luca"));
        return plan ? [plan.status, ...new Set(plan.steps.map((s) => s.status))].join(",") : "";
      })
      .toBe("cancelled,skipped");
    await api.dispose();
  });
});

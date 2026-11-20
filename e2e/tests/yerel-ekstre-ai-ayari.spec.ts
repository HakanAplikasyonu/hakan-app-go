import { expect, test } from "@playwright/test";

import { Api, DEMO, officePath, uiLogin } from "./lib";

/**
 * Ekstrede AI kolon önerisi ofis ayarıyla kapatılır (#669). Kapalıyken
 * tanınmayan düzendeki dosyanın analizi kural tabanlı akışla yine çalışır
 * ve hiçbir kolon AI kaynaklı gelmez. Sonda ayar eski haline (açık) döner.
 */
const csv = Buffer.from(
  "Gün;Metin;Para;Son Durum\n01.10.2026;FAST GELEN E2E;500,00;1.500,00\n",
  "utf8",
);

type Settings = { settings: Record<string, unknown> };
type Analysis = { columns: { header: string; origin?: string }[] };

test("ekstre AI kolon önerisi ofis ayarıyla kapatılır", async ({ page }) => {
  const api = await Api.login(DEMO.owner);
  await uiLogin(page, DEMO.owner);
  await page.goto(officePath("/settings"));
  const toggle = page.getByRole("switch", { name: "Tanınmayan ekstrede kolonları yapay zekâ önersin" });
  await expect(toggle).toBeChecked();

  await toggle.click();
  await expect(toggle).not.toBeChecked();
  await expect(page.getByText("Ayarlar kaydedildi.").first()).toBeVisible();
  const off = await api.json<Settings>(await api.get("/v1/tenant/settings"));
  expect(off.settings.bank_statement_ai).toBe(false);

  // Kapalıyken analiz kural tabanlı çalışır; AI kaynaklı kolon yok.
  const a = await api.json<Analysis>(
    await api.upload("/v1/tenant/bank-statement-templates/analyze", {
      name: "e2e-ai-kapali.csv",
      mimeType: "text/csv",
      buffer: csv,
    }),
  );
  expect(a.columns.map((c) => c.header)).toEqual(["Gün", "Metin", "Para", "Son Durum"]);
  expect(a.columns.filter((c) => c.origin === "ai")).toHaveLength(0);

  // Eski haline: açık.
  await toggle.click();
  await expect(toggle).toBeChecked();
  await expect
    .poll(async () => (await api.json<Settings>(await api.get("/v1/tenant/settings"))).settings.bank_statement_ai)
    .toBe(true);
  await api.dispose();
});

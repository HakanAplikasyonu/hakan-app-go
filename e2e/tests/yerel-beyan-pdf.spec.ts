import { readFileSync } from "node:fs";

import { expect, test } from "@playwright/test";

import { Api, DEMO, engagementOf, officePath, uiLogin } from "./lib";

/**
 * Beyan özeti PDF'i ve ofis beyan tablosunun Excel'i (#669 H).
 *
 * Veri: demo ofisi, Atlas Gıda; yalnız okur (beyan kaydı değiştirmez).
 */
test.describe("beyan özeti PDF ve ofis Excel (#669 H)", () => {
  test("mükellef beyan ekranından PDF özet iner; uç biçimi denetler", async ({ page }) => {
    const api = await Api.login(DEMO.owner);
    const engagement = await engagementOf(api, DEMO.atlas.title);

    await uiLogin(page, DEMO.owner);
    await page.goto(officePath(`/engagements/${engagement}/tax-returns`));
    await expect(page.getByRole("region", { name: "Beyan durumu" })).toBeVisible();
    const [pdf] = await Promise.all([page.waitForEvent("download"), page.getByRole("button", { name: "PDF özet" }).click()]);
    expect(pdf.suggestedFilename()).toMatch(/^kdv1-\d{4}-\d{2}\.pdf$/);
    expect(readFileSync(await pdf.path()).subarray(0, 4).toString("latin1")).toBe("%PDF");

    // Uç: format=pdf PDF, varsayılan Excel, bilinmeyen biçim 400.
    const period = pdf.suggestedFilename().slice(5, 12);
    const base = `/v1/tenant/engagements/${engagement}/tax-returns`;
    for (const kind of ["kdv1", "muhsgk", "babs"]) {
      const res = await api.get(`${base}/${kind}/${period}/export?format=pdf`);
      expect(res.status(), await res.text()).toBe(200);
      expect(res.headers()["content-type"]).toContain("application/pdf");
      expect(res.headers()["content-disposition"]).toContain(`${kind}-${period}.pdf`);
      expect((await res.body()).subarray(0, 4).toString("latin1")).toBe("%PDF");
    }
    const xlsx = await api.get(`${base}/kdv1/${period}/export`);
    expect(xlsx.headers()["content-type"]).toContain("spreadsheetml");
    expect((await api.get(`${base}/kdv1/${period}/export?format=docx`)).status()).toBe(400);
    await api.dispose();
  });

  test("ofis beyan tablosu Excel olarak iner", async ({ page }) => {
    await uiLogin(page, DEMO.owner);
    await page.goto(officePath("/tax-returns"));
    await expect(page.getByRole("row", { name: new RegExp(DEMO.atlas.title) })).toBeVisible();
    const [xlsx] = await Promise.all([page.waitForEvent("download"), page.getByRole("button", { name: "Excel indir" }).click()]);
    expect(xlsx.suggestedFilename()).toMatch(/^beyannameler_\d{4}-\d{2}\.xlsx$/);
    expect(readFileSync(await xlsx.path()).subarray(0, 2).toString("latin1")).toBe("PK");
  });
});

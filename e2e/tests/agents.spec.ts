import { randomUUID } from "node:crypto";

import { expect, test } from "@playwright/test";

import { Api, DEMO, officePath, uiLogin } from "./lib";

/**
 * Masaüstü ajanları sayfası (#285): GET /v1/tenant/agents listesi, durum
 * rozeti, son hata, kaldırma ve genel bakıştaki çevrimdışı kartı.
 *
 * Gerçek kayıt API'den yapılır (masaüstü uygulamasının yaptığı gibi). Sunucu
 * ajanı ancak 30 dakika sessizlikten sonra çevrimdışı yaptığı için çevrimdışı
 * hâl, liste yanıtı taklit edilerek denetlenir.
 * Bu dosya yalnızca kendi kaydettiği ajanlara dokunur.
 */
test.describe.configure({ mode: "serial" });

const AGENTS_API = "**/api/v1/tenant/agents";

test("kayıtlı ajan listelenir ve kaldırılabilir", async ({ page }) => {
  const api = await Api.login(DEMO.owner);
  const name = `E2E-PC-${randomUUID().slice(0, 8)}`;
  const res = await api.post("/v1/tenant/agents", {
    machine_id: `e2e-${randomUUID()}`,
    name,
    platform: "windows",
    version: "0.1.0",
  });
  expect(res.status(), await res.text()).toBe(200);
  await api.dispose();

  await uiLogin(page, DEMO.owner);
  await page.goto(officePath());
  await page.getByRole("link", { name: "Masaüstü ajanları" }).first().click();
  await expect(page).toHaveURL(new RegExp(`${officePath("/agents")}$`));

  const row = page.getByRole("row").filter({ hasText: name });
  await expect(row).toBeVisible();
  await expect(row).toContainText("Windows");
  await expect(row).toContainText("0.1.0");
  await expect(row).toContainText("Çevrimiçi");
  await expect(row.locator("time")).toBeVisible();

  await row.getByRole("button", { name: "Kaldır" }).click();
  await page.getByRole("alertdialog").or(page.getByRole("dialog")).getByRole("button", { name: "Kaldır" }).click();
  await expect(page.getByRole("row").filter({ hasText: name })).toHaveCount(0);
});

test("çevrimdışı ajan: genel bakış kartı ve son hata", async ({ page }) => {
  const now = Date.now();
  const iso = (minAgo: number) => new Date(now - minAgo * 60_000).toISOString();
  await page.route(AGENTS_API, (route) =>
    route.request().method() === "GET"
      ? route.fulfill({
          json: {
            success: true,
            data: {
              items: [
                {
                  uuid: randomUUID(),
                  machine_id: "e2e-offline-0001",
                  name: "Muhasebe-PC-02",
                  platform: "macos",
                  version: "0.1.0",
                  status: "offline",
                  last_seen_at: iso(95),
                  offline_since: iso(65),
                  heartbeat: {},
                  created_at: iso(5000),
                  last_error: { code: "AGENT_OFFLINE", at: iso(65) },
                  heartbeat_seconds: 300,
                },
                {
                  uuid: randomUUID(),
                  machine_id: "e2e-online-0001",
                  name: "Muhasebe-PC-01",
                  platform: "windows",
                  version: "0.1.0",
                  status: "online",
                  last_seen_at: iso(2),
                  offline_since: null,
                  heartbeat: {},
                  created_at: iso(5000),
                  last_error: null,
                  heartbeat_seconds: 300,
                },
              ],
            },
            meta: { request_id: "e2e" },
          },
        })
      : route.continue(),
  );

  await uiLogin(page, DEMO.owner);
  await page.goto(officePath());
  const card = page.getByRole("status").filter({ hasText: "masaüstü ajanı çevrimdışı" });
  await expect(card).toContainText("1 masaüstü ajanı çevrimdışı");
  await expect(card).toContainText("Muhasebe-PC-02");
  await expect(card).not.toContainText("Muhasebe-PC-01");

  await card.getByRole("link", { name: "Ajanları gör" }).click();
  await expect(page).toHaveURL(new RegExp(`${officePath("/agents")}$`));
  const offline = page.getByRole("row").filter({ hasText: "Muhasebe-PC-02" });
  await expect(offline).toContainText("macOS");
  await expect(offline).toContainText("Çevrimdışı");
  await expect(offline).toContainText("Bağlantı koptu");
});

test("genel bakış: çevrimdışı ajan yoksa kart görünmez", async ({ page }) => {
  await page.route(AGENTS_API, (route) =>
    route.fulfill({ json: { success: true, data: { items: [] }, meta: { request_id: "e2e" } } }),
  );
  await uiLogin(page, DEMO.owner);
  await page.goto(officePath());
  await expect(page.getByText(DEMO.atlas.title).first()).toBeVisible();
  await expect(page.getByText("masaüstü ajanı çevrimdışı")).toHaveCount(0);
});

test("kılavuz bağlantısı /t?to=agents ofisin ajan sayfasını açar", async ({ page }) => {
  await uiLogin(page, DEMO.owner);
  await page.goto("/yardim/masaustu");
  await page.getByRole("link", { name: "ajanları aç" }).first().click();
  await expect(page).toHaveURL(new RegExp(`${officePath("/agents")}$`));
  await expect(page.getByRole("heading", { name: "Masaüstü ajanları" })).toBeVisible();
});

import { expect, test } from "@playwright/test";

import {
  addConsentedTaxpayer,
  Api,
  DEMO,
  officePath,
  uiLogin,
  uniqueVkn,
} from "./lib";

/**
 * İşNet/EDM ilk bağlantıda önce son iki ay, canlı ilerleme (#780).
 *
 * - İşNet kullanıcı istemez: bağlantı açılınca ilk yükleme (backfill) hemen
 *   kuyruğa girer ve bağlantı görünümünde `progress` gelir (sırada, son iki
 *   ay, geçmiş aylar ya da bitti). Geçmiş ay sayısı geçen yılın Ocak'ından
 *   bu yana (son iki ay hariç) en az 10'dur.
 * - Bağlantılar sekmesindeki İşNet kartı ilerlemeyi Türkçe gösterir
 *   ("İlk çekim sıraya alındı", "Bu ay: N fatura geldi …", "geçmiş aylar
 *   çekiliyor (x/y)") ya da işçi entegratöre ulaşamadıysa Türkçe hatayı.
 * - Son iki ay hazır olunca "Faturalara git" mükellefin fatura listesine
 *   götürür.
 *
 * Sıralama (bu ay → geçen ay → geçmiş, son iki ayın faturaları geçmiş
 * listelenmeden önce) sahte entegratörle Go testinde doğrulanır
 * (entegrator/hizli_ilk_db_test.go, plan_test.go); bu koşuda gerçek İşNet
 * kullanıcısı / sabit IP yok.
 */
test.describe.configure({ mode: "serial" });

const run = `${Date.now()}`.slice(-6);
const taxpayer = { vkn: uniqueVkn(), title: `Hızlı İlk Yükleme ${run} A.Ş.` };

type Progress = {
  phase: "queued" | "recent" | "history" | "done";
  recent_ready: boolean;
  this_month_documents: number;
  this_month_invoices: number;
  history_months_done: number;
  history_months_total: number;
};
type Conn = {
  uuid: string;
  integrator_id: string;
  state: string;
  error?: { message: string };
  progress?: Progress;
};

test.describe("İşNet/EDM ilk bağlantı: önce son iki ay (#780)", () => {
  let engagement = "";
  let api: Api;
  const base = () =>
    `/v1/tenant/engagements/${engagement}/einvoice-connections`;

  test.beforeAll(async ({ browser }) => {
    test.setTimeout(240_000);
    api = await Api.login(DEMO.owner);
    engagement = await addConsentedTaxpayer(browser, api, taxpayer, "hizli");
  });

  test.afterAll(async () => {
    await api?.dispose();
  });

  test("İşNet bağlanınca ilk yükleme ilerlemesi gelir", async () => {
    const res = await api.post(base(), {
      integrator_id: "isnet",
      environment: "test",
    });
    expect(res.status()).toBe(201);
    const created = await api.json<Conn>(res);
    expect(created.state).toBe("waiting");
    expect(created.progress?.phase).toBe("queued");
    expect(created.progress?.history_months_total).toBeGreaterThanOrEqual(10);

    const list = await api.json<{ items: Conn[] }>(await api.get(base()));
    const conn = list.items.find((c) => c.integrator_id === "isnet")!;
    if (conn.progress) {
      const p = conn.progress;
      expect(["queued", "recent", "history", "done"]).toContain(p.phase);
      expect(p.history_months_done).toBeLessThanOrEqual(p.history_months_total);
      expect(p.this_month_invoices).toBeLessThanOrEqual(p.this_month_documents);
      if (p.phase === "history" || p.phase === "done") {
        expect(p.recent_ready).toBe(true);
      }
    } else {
      // İlk yükleme entegratöre ulaşamadı: hata gösterilir.
      expect(conn.error?.message ?? "").not.toBe("");
    }
  });

  test("kart ilerlemeyi Türkçe gösterir; hazırsa faturalara gider", async ({
    page,
  }) => {
    test.setTimeout(120_000);
    await uiLogin(page, DEMO.owner);
    await page.goto(officePath(`/engagements/${engagement}/connections`));
    const card = page.locator("article").filter({ hasText: "İşNet" });
    await expect(card).toBeVisible();

    const progress = card.getByTestId("einvoice-progress");
    const failed = card.getByRole("alert");
    await expect(progress.or(failed).first()).toBeVisible({ timeout: 30_000 });
    if (await progress.isVisible()) {
      await expect(progress).toContainText(
        /İlk çekim sıraya alındı|Bu ay: \d+ fatura|İlk yükleme tamamlandı/,
      );
      await expect(progress).not.toContainText(/backfill|queued|history/i);
      const go = card.getByTestId("einvoice-go-invoices");
      if (await go.isVisible()) {
        await go.click();
        await expect(page).toHaveURL(
          new RegExp(`/engagements/${engagement}/invoices`),
        );
      }
    } else {
      await expect(failed).not.toContainText(/provider|soap fault/i);
    }
  });
});

import { expect, test } from "@playwright/test";

import { Api, DEMO } from "./lib";

/**
 * Mesajlarım (#780): mükellef kendisine giden SMS ve e-postaları
 * `GET /v1/taxpayer/messages` ile görür (mobil "Mesajlarım" ekranının ucu).
 *
 *   demo mükellef kullanıcısı (Atlas Gıda) listeyi alır → mesajlar var,
 *   sayfalı (limit) → ofisin iç alanları (gönderen, sağlayıcı iş numarası,
 *   hata kodu, kaynak kural) yok, adres maskeli, durum kod → ofis
 *   kullanıcısı X-Taxpayer olmadan bu uca giremez.
 */
type Message = Record<string, unknown> & { channel: string; status: string; to: string; body: string };

test("mükellef kendisine giden mesajları görür, iç alanlar dönmez", async () => {
  const tp = await Api.login(DEMO.taxpayerUser);
  const me = await tp.get("/v1/auth/me");
  expect(me.status()).toBe(200);
  const taxpayers = ((await me.json()).data.taxpayers ?? []) as { uuid: string; title: string }[];
  const atlas = taxpayers.find((t) => t.title === DEMO.atlas.title) ?? taxpayers[0];
  expect(atlas, "demo mükellef kullanıcısının şirketi").toBeTruthy();

  const res = await tp.get("/v1/taxpayer/messages?limit=3", { "X-Taxpayer": atlas.uuid });
  expect(res.status(), await res.text()).toBe(200);
  const page = (await res.json()).data as { items: Message[]; total: number; limit: number };
  expect(page.limit).toBe(3);
  expect(page.items.length).toBeGreaterThan(0);
  expect(page.items.length).toBeLessThanOrEqual(3);
  expect(page.total).toBeGreaterThanOrEqual(page.items.length);
  for (const m of page.items) {
    expect(["sms", "email"]).toContain(m.channel);
    expect(["pending", "sent", "delivered", "failed"]).toContain(m.status);
    expect(m.body.length).toBeGreaterThan(0);
    expect(m.to).toContain("*");
    for (const key of ["provider_job_id", "sent_by", "sent_by_name", "error_code", "error_message", "source_rule", "source_ref", "msgheader", "opened_at"]) {
      expect(m, `iç alan ${key}`).not.toHaveProperty(key);
    }
  }
  await tp.dispose();

  // Ofis kullanıcısı mükellef ucuna giremez.
  const office = await Api.login(DEMO.owner);
  const denied = await office.get("/v1/taxpayer/messages");
  expect(denied.status()).toBeGreaterThanOrEqual(400);
  await office.dispose();
});

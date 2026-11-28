import { readFileSync } from "node:fs";
import { join } from "node:path";

import { expect, test } from "@playwright/test";

import { Api, DEMO, engagementOf } from "./lib";

/**
 * WhatsApp uzantısı "Banka ekstresi olarak kaydet" (#780): uzantının kayıt
 * ucu (`POST …/whatsapp/captures`, `target=bank_statement`) dosyayı panelin
 * ekstre içe aktarımına verir. Uzantının çağrıları sırasıyla: banka
 * hesapları (`GET …/cash-boxes`), seçilen hesapla dosya; yanıttaki özet
 * "N satır içe aktarıldı" olarak gösterilir.
 *
 * Veri: demo ofisi, Atlas Gıda. Her koşu yeni bir banka hesabı açar (hareket
 * tekrarı hesap başına denetlendiği için koşular birbirini etkilemez) ve
 * sonunda içe aktarımı geri alır; hesap listede kalır.
 */
const FIXTURE = join(__dirname, "..", "fixtures", "whatsapp-ekstre.csv");

test("CSV ekstre seçilen hesaba aktarılır, tekrarı yeni satır eklemez, tanınmayan biçim panele yönlendirilir", async () => {
  const api = await Api.login(DEMO.owner);
  const eng = await engagementOf(api, DEMO.atlas.title);
  const base = `/v1/tenant/engagements/${eng}`;

  const account = String(Date.now()).slice(-8);
  const box = await api.json<{ uuid: string }>(
    await api.post(`${base}/cash-boxes`, { bank_account_number: account, name: `WhatsApp e2e ${account}` }),
  );
  // Uzantının listesi: kullanımdaki banka hesapları, yeni hesap içinde.
  const boxes = await api.json<{ items: { uuid: string; type: string; active: boolean }[] }>(await api.get(`${base}/cash-boxes`));
  expect(boxes.items.find((b) => b.uuid === box.uuid)).toMatchObject({ type: "bank_account", active: true });

  const csv = { name: "ekstre-kasim.csv", mimeType: "text/csv", buffer: readFileSync(FIXTURE) };
  const fields = { target: "bank_statement", chat_key: "905551112233@c.us", cash_box_uuid: box.uuid };

  const first = await api.postForm(`${base}/whatsapp/captures`, { ...fields, file: csv });
  expect(first.status(), await first.text()).toBe(201);
  const saved = await api.json<{
    target: string;
    replayed: boolean;
    taxpayer: { title: string };
    bank_statement: { import_id: string; format: string; lines: number; inserted: number };
  }>(first);
  expect(saved.target).toBe("bank_statement");
  expect(saved.taxpayer.title).toBe(DEMO.atlas.title);
  expect(saved.bank_statement).toMatchObject({ format: "csv", lines: 3, inserted: 3 });
  expect(saved.replayed).toBe(false);

  // Aynı dosya ikinci kez: yeni satır yok.
  const again = await api.json<{ replayed: boolean; bank_statement: { inserted: number; skipped: number } }>(
    await api.postForm(`${base}/whatsapp/captures`, { ...fields, file: csv }),
  );
  expect(again.replayed).toBe(true);
  expect(again.bank_statement).toMatchObject({ inserted: 0, skipped: 3 });

  // Hareketler panelin banka listesinde, seçilen hesapta.
  const txs = await api.json<{ items: { cash_box_name: string }[] }>(
    await api.get(`${base}/bank-transactions?cash_box_uuid=${box.uuid}&limit=100`),
  );
  expect(txs.items.map((t) => t.cash_box_name)).toEqual(Array(3).fill(`WhatsApp e2e ${account}`));

  // Sütunları tanınmayan dosya: uzantı panelden içe aktarmayı söyler.
  const unknown = await api.postForm(`${base}/whatsapp/captures`, {
    ...fields,
    file: { name: "baska.csv", mimeType: "text/csv", buffer: Buffer.from("Gün;Metin;Para\n01.11.2026;X;5,00\n", "utf8") },
  });
  expect(unknown.status()).toBe(422);
  expect((await unknown.json()).error.code).toBe("STATEMENT_LAYOUT_UNKNOWN");

  // Fotoğraf ekstre değildir.
  const photo = await api.postForm(`${base}/whatsapp/captures`, {
    ...fields,
    file: { name: "ekstre.jpg", mimeType: "image/jpeg", buffer: Buffer.from([0xff, 0xd8, 0xff, 0xe0, 0, 0x10, 0x4a, 0x46, 0x49, 0x46, 0]) },
  });
  expect(photo.status()).toBe(400);

  // Geri alma panelinkiyle aynı.
  const undo = await api.json<{ deleted: number }>(await api.delete(`${base}/bank-statements/${saved.bank_statement.import_id}`));
  expect(undo.deleted).toBe(3);
  await api.dispose();
});

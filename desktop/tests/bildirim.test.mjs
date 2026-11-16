// Bildirim kuralları (#325): node --test tests/bildirim.test.mjs
import { test } from "node:test";
import assert from "node:assert/strict";

import { AG_ESIGI_MS, degerlendir, yeniDurum } from "../ui/bildirim.js";

const at = (h, m = 0) => new Date(2026, 10, 16, h, m);

test("başarılı gönderimde bildirim yok", () => {
  const s = yeniDurum();
  assert.deepEqual(degerlendir(s, { agSorunu: false, kaliciHatalar: [] }, at(10)), []);
});

test("ağ hatası 30 dk sürerse tek bildirim, dönünce sıfırlanır", () => {
  const s = yeniDurum();
  const g = { agSorunu: true, kaliciHatalar: [] };
  assert.equal(degerlendir(s, g, at(10, 0)).length, 0);
  assert.equal(degerlendir(s, g, at(10, 29)).length, 0);
  const n = degerlendir(s, g, new Date(at(10, 0).getTime() + AG_ESIGI_MS));
  assert.equal(n.length, 1);
  assert.match(n[0].baslik, /30 dakikadır bağlantı yok/);
  assert.equal(degerlendir(s, g, at(11, 30)).length, 0, "ikinci bildirim yok");
  degerlendir(s, { agSorunu: false, kaliciHatalar: [] }, at(11, 31));
  assert.equal(degerlendir(s, g, at(11, 32)).length, 0);
  assert.equal(degerlendir(s, g, at(12, 2)).length, 1, "yeni kesinti yeniden sayılır");
});

test("kalıcı hatalar gün sonunda tek özet", () => {
  const s = yeniDurum();
  const g = { agSorunu: false, kaliciHatalar: [{ anahtar: "a/tarama.pdf" }, { anahtar: "a/bozuk.xml" }] };
  assert.equal(degerlendir(s, g, at(9)).length, 0, "gün içinde bildirim yok");
  const n = degerlendir(s, g, at(18, 5));
  assert.equal(n.length, 1);
  assert.match(n[0].baslik, /bugün 2 dosya gönderilemedi/);
  assert.equal(degerlendir(s, g, at(19)).length, 0, "aynı gün ikinci özet yok");
});

test("gün sonunda yeni kalıcı hata yoksa özet yok", () => {
  const s = yeniDurum();
  assert.equal(degerlendir(s, { agSorunu: false, kaliciHatalar: [] }, at(18, 30)).length, 0);
});

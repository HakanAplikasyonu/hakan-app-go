import assert from 'node:assert/strict';
import { test } from 'node:test';

import { baslik, durum, kanalAdi, onizleme, zaman } from './ozet.ts';

test('durum adları', () => {
  assert.equal(durum('delivered').metin, 'İletildi');
  assert.equal(durum('sent').metin, 'Gönderildi');
  assert.equal(durum('failed').ton, 'danger');
  assert.equal(durum('bilinmeyen').metin, 'Gönderiliyor');
});

test('kanal adı', () => {
  assert.equal(kanalAdi('email'), 'E-posta');
  assert.equal(kanalAdi('sms'), 'SMS');
});

test('başlık: e-postada konu, SMS’te ilk satır', () => {
  assert.equal(baslik({ channel: 'email', subject: 'Ekim KDV', body: 'Sayın…' }), 'Ekim KDV');
  assert.equal(baslik({ channel: 'sms', subject: '', body: 'KDV son gün 28 Kasım.\nTutar 18.450 TL' }), 'KDV son gün 28 Kasım.');
  assert.equal(baslik({ channel: 'email', subject: ' ', body: '' }), 'E-posta');
  const uzun = 'ş'.repeat(80);
  assert.equal(Array.from(baslik({ channel: 'sms', subject: '', body: uzun })).length, 60);
});

test('önizleme tek satır ve kısa', () => {
  assert.equal(onizleme('Sayın Atlas,\n\nKDV verildi.'), 'Sayın Atlas, KDV verildi.');
  const p = onizleme('a '.repeat(100));
  assert.ok(p.endsWith('…'));
  assert.ok(Array.from(p).length <= 90);
});

test('zaman biçimi', () => {
  const d = new Date(2026, 10, 19, 9, 5);
  assert.equal(zaman(d.toISOString()), '19 Kasım 2026 09:05');
  assert.equal(zaman(null), '');
  assert.equal(zaman('bozuk'), '');
});

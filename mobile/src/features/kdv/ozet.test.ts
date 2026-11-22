import assert from 'node:assert/strict';
import { test } from 'node:test';

import { alindiMetni, ayAdi, kdvGizli, kdvSonucu, sonGun, tarih, tl, yuklemeHatasi } from './ozet.ts';

test('tl biçimi', () => {
  assert.equal(tl('1234.5'), '₺1.234,50');
  assert.equal(tl('0'), '₺0,00');
  assert.equal(tl('-1500000'), '-₺1.500.000,00');
  assert.equal(tl('abc'), '₺0,00');
  assert.equal(tl(null), '₺0,00');
});

test('tarih ve ay adı', () => {
  assert.equal(tarih('2026-12-26'), '26 Aralık 2026');
  assert.equal(tarih('2026-01-05T10:00:00Z'), '5 Ocak 2026');
  assert.equal(tarih(null), '');
  assert.equal(ayAdi('2026-11'), 'Kasım 2026');
});

test('son gün satırı', () => {
  assert.deepEqual(sonGun({ due_date: '2026-12-26', days_left: 4 }), {
    metin: 'Son ödeme günü 26 Aralık 2026 · 4 gün kaldı',
    gecti: false,
    yakin: false,
  });
  assert.ok(sonGun({ due_date: '2026-12-26', days_left: 0 }).metin.endsWith('bugün son gün'));
  assert.equal(sonGun({ due_date: '2026-12-26', days_left: 2 }).yakin, true);
  assert.equal(sonGun({ due_date: '2026-12-26', days_left: -1 }).gecti, true);
});

test('ödenecek ya da devreden', () => {
  assert.deepEqual(kdvSonucu({ payable: '1200.00', carry_out: '0' }), { etiket: 'Ödenecek KDV (tahmini)', tutar: '₺1.200,00', odenecek: true });
  assert.deepEqual(kdvSonucu({ payable: '0', carry_out: '300' }), { etiket: 'Sonraki aya devredecek KDV', tutar: '₺300,00', odenecek: false });
  assert.equal(kdvSonucu({ payable: '0', carry_out: '0' }).odenecek, true);
});

test('KDV kartı gizleme', () => {
  assert.equal(kdvGizli({ status: 403, code: 'TAXPAYER_VAT_PANEL_DISABLED' }), true);
  assert.equal(kdvGizli({ status: 404, code: 'NOT_FOUND' }), true);
  assert.equal(kdvGizli({ status: 500, code: 'INTERNAL' }), false);
  assert.equal(kdvGizli(null), false);
});

test('yabancı fatura metinleri', () => {
  assert.equal(alindiMetni(3), 'Bu ay 3 yabancı faturanız alındı');
  assert.equal(alindiMetni(0), 'Bu ay henüz yabancı fatura göndermediniz.');
  assert.match(yuklemeHatasi({ status: 409, code: 'CONFLICT' }), /kapattı/);
  assert.match(yuklemeHatasi({ status: 409, code: 'NO_ACTIVE_ENGAGEMENT' }), /ofisi yok/);
  assert.match(yuklemeHatasi({ status: 400 }), /PDF/);
  assert.match(yuklemeHatasi(new Error('x')), /gönderilemedi/);
});

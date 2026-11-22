import assert from 'node:assert/strict';
import { test } from 'node:test';

import { amountLabel, parseRate, rateLabel, rowSubtitle, sourceLabel, statusLabel, totalsSummary, treatmentLabel } from './format.ts';

const base = {
  amount: '1000',
  currency: 'USD',
  treatment: 'kdv2' as const,
  kdv2_try: '7650',
  stopaj_try: '0',
  status: 'draft' as const,
  missing_rate: false,
  exchange_rate: '38.25',
  exchange_rate_source: 'tcmb' as const,
  exchange_rate_date: '2026-11-20',
  source_kind: null,
};

test('amountLabel uses TL sign or currency code', () => {
  assert.equal(amountLabel('1234.5'), '1.234,50 ₺');
  assert.equal(amountLabel('1000', 'USD'), '1.000,00 USD');
  assert.equal(amountLabel(null), '—');
});

test('labels', () => {
  assert.equal(treatmentLabel('kdv2'), 'Yalnız KDV2');
  assert.equal(treatmentLabel('kdv2_stopaj'), 'KDV2 + stopaj');
  assert.equal(statusLabel('confirmed'), 'Onaylandı');
  assert.equal(statusLabel('draft'), 'Taslak');
  assert.equal(sourceLabel('taxpayer_upload'), 'Mükelleften geldi');
  assert.equal(sourceLabel('whatsapp'), 'Mükelleften geldi');
  assert.equal(sourceLabel(null), null);
});

test('rateLabel', () => {
  assert.equal(rateLabel(base), 'TCMB kuru 38,25 · 20.11.2026');
  assert.equal(rateLabel({ ...base, exchange_rate_source: 'manual' }), 'Elle girilen kur 38,25');
  assert.equal(rateLabel({ ...base, exchange_rate: null, missing_rate: true }), 'Kur eksik');
  assert.equal(rateLabel({ ...base, currency: 'TRY' }), null);
});

test('rowSubtitle shows stopaj only for kdv2_stopaj', () => {
  assert.equal(rowSubtitle(base), '1.000,00 USD · Yalnız KDV2 · KDV2 7.650,00 ₺');
  assert.equal(
    rowSubtitle({ ...base, treatment: 'kdv2_stopaj', stopaj_try: '6750' }),
    '1.000,00 USD · KDV2 + stopaj · KDV2 7.650,00 ₺ · Stopaj 6.750,00 ₺',
  );
  assert.match(rowSubtitle({ ...base, kdv2_try: null }), /KDV2 —/);
});

test('totalsSummary', () => {
  assert.equal(totalsSummary({ count: 0, confirmed: 0, draft: 0, missing_rate: 0 }), 'Bu dönemde yabancı fatura yok.');
  assert.equal(
    totalsSummary({ count: 3, confirmed: 1, draft: 2, missing_rate: 1 }),
    '3 fatura · 1 onaylı · 2 taslak · 1 faturada kur eksik (toplama girmedi)',
  );
});

test('parseRate', () => {
  assert.equal(parseRate('38,25'), '38.25');
  assert.equal(parseRate(' 41.1234 '), '41.1234');
  assert.equal(parseRate('0'), null);
  assert.equal(parseRate('abc'), null);
  assert.equal(parseRate('1.234,5'), null);
  assert.equal(parseRate(''), null);
});

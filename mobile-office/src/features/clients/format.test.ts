import assert from 'node:assert/strict';
import { test } from 'node:test';

import { clientSubtitle, mailHref, money, normalizeSearch, recentPeriods, telHref, initials } from './format.ts';

test('normalizeSearch', () => {
  assert.equal(normalizeSearch(' a '), undefined);
  assert.equal(normalizeSearch('  Atlas   Gıda '), 'Atlas Gıda');
  assert.equal(normalizeSearch('x'.repeat(150))?.length, 100);
});

test('tel and mail links', () => {
  assert.equal(telHref('+90 (212) 555-01 01'), 'tel:+902125550101');
  assert.equal(telHref('12'), null);
  assert.equal(telHref(null), null);
  assert.equal(mailHref('okan@atlas.test'), 'mailto:okan@atlas.test');
  assert.equal(mailHref('not-an-email'), null);
});

test('recentPeriods crosses the year in Istanbul time', () => {
  assert.deepEqual(recentPeriods(new Date('2026-11-16T12:00:00Z')), ['2026-11', '2026-10']);
  // 31 Dec 22:30 UTC is already 1 Jan in Istanbul.
  assert.deepEqual(recentPeriods(new Date('2026-12-31T22:30:00Z')), ['2027-01', '2026-12']);
});

test('money and subtitle', () => {
  assert.equal(money('1234.5'), '1.234,50 ₺');
  assert.equal(money('10', 'USD'), '10,00 USD');
  assert.equal(money('x'), 'x');
  assert.equal(
    clientSubtitle({ tax_id: '1234567890', kind: 'legal', open_warnings: 3, open_blockers: 1, open_tasks: 2 }),
    'VKN 1234567890 · 1 engelleyici · 2 açık görev',
  );
  assert.equal(clientSubtitle({ tax_id: '1', kind: 'individual', open_warnings: 2, open_blockers: 0, open_tasks: 0 }), 'TCKN 1 · 2 uyarı');
});

test('initials skip company suffixes', () => {
  assert.equal(initials('Atlas Gıda Ticaret A.Ş.'), 'AG');
  assert.equal(initials('Kuzey Lojistik Ltd. Şti.'), 'KL');
  assert.equal(initials('ışık'), 'I');
  assert.equal(initials(''), '?');
});

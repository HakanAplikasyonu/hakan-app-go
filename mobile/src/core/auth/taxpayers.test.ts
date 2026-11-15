import assert from 'node:assert/strict';
import { test } from 'node:test';

import { initials, pickTaxpayer, sortTaxpayers, type TaxpayerOption } from './taxpayers.ts';

const a: TaxpayerOption = { uuid: 'a', kind: 'legal', title: 'Yıldız Market Ltd. Şti.', role: 'owner' };
const b: TaxpayerOption = { uuid: 'b', kind: 'individual', title: 'Çınar Kırtasiye', role: 'employee' };
const c: TaxpayerOption = { uuid: 'c', kind: 'legal', title: 'Ankara Yapı A.Ş.', role: 'owner' };

test('last used company opens again while the membership holds', () => {
  assert.equal(pickTaxpayer([a, b], 'b'), 'b');
});

test('a dropped membership falls back to the picker', () => {
  assert.equal(pickTaxpayer([a, b], 'gone'), null);
});

test('a single company opens directly', () => {
  assert.equal(pickTaxpayer([a], null), 'a');
  assert.equal(pickTaxpayer([a], 'gone'), 'a');
});

test('no company, nothing to open', () => {
  assert.equal(pickTaxpayer([], null), null);
});

test('picker is sorted in Turkish order', () => {
  assert.deepEqual(sortTaxpayers([a, b, c]).map((t) => t.uuid), ['c', 'b', 'a']);
});

test('initials skip company suffixes', () => {
  assert.equal(initials('Yıldız Market Ltd. Şti.'), 'YM');
  assert.equal(initials('ibrahim'), 'İ');
  assert.equal(initials('Ankara Yapı A.Ş.'), 'AY');
});

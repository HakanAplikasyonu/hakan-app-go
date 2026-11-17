import assert from 'node:assert/strict';
import { test } from 'node:test';

import { balanceLabel, balanceSpoken, changeOf, filterLines, isBalanced, shareText, trialBalanceLabel } from './format.ts';

test('trialBalanceLabel uses the period, else the upload date', () => {
  assert.equal(trialBalanceLabel({ period: '2026-10', created_at: '2026-11-03T09:00:00Z' }), 'Ekim 2026');
  assert.equal(trialBalanceLabel({ period: null, created_at: '2026-11-03T09:00:00Z' }), '03.11.2026 yüklemesi');
});

test('balanceLabel marks borç / alacak', () => {
  assert.equal(balanceLabel('7150'), '7.150,00 ₺ B');
  assert.equal(balanceLabel('-35500.5'), '35.500,50 ₺ A');
  assert.equal(balanceLabel('0.00'), '0,00 ₺');
  assert.equal(balanceSpoken('-10'), 'alacak bakiyesi 10,00 lira');
});

test('changeOf', () => {
  assert.equal(changeOf({ balance: '1', previous_balance: null, change: null }), null);
  assert.deepEqual(changeOf({ balance: '1000', previous_balance: '600', change: '400' }), { text: '+400,00 ₺', percent: '%66,7', tone: 'up' });
  assert.deepEqual(changeOf({ balance: '-900', previous_balance: null, change: '-900' }), { text: 'Yeni hesap', percent: null, tone: 'new' });
  assert.deepEqual(changeOf({ balance: '5', previous_balance: '5', change: '0' }), { text: 'Değişmedi', percent: null, tone: 'flat' });
  assert.deepEqual(changeOf({ balance: '-300', previous_balance: '0', change: '-300' }), { text: '−300,00 ₺', percent: null, tone: 'down' });
});

test('filterLines by code prefix or Turkish name', () => {
  const lines = [
    { code: '100', name: 'KASA' },
    { code: '320.01', name: 'YURTİÇİ SATICILAR' },
  ];
  assert.equal(filterLines(lines, '32').length, 1);
  assert.equal(filterLines(lines, 'yurtiçi')[0].code, '320.01');
  assert.equal(filterLines(lines, ' ').length, 2);
});

test('isBalanced and shareText', () => {
  assert.equal(isBalanced({ debit_total: '100.00', credit_total: '100.00' }), true);
  assert.equal(isBalanced({ debit_total: '100.00', credit_total: '99.00' }), false);
  const text = shareText('Atlas Gıda', 'Ekim 2026', { debit_total: '10', credit_total: '10' }, [
    { code: '100', name: 'KASA', level: 1, balance: '10' },
    { code: '100.01', name: 'MERKEZ', level: 2, balance: '10' },
  ], 'Eylül 2026');
  assert.match(text, /Ekim 2026 mizanı/);
  assert.match(text, /100 KASA: 10,00 ₺ B/);
  assert.doesNotMatch(text, /MERKEZ/);
  assert.match(text, /Karşılaştırma: Eylül 2026/);
});

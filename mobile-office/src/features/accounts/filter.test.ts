import assert from 'node:assert/strict';
import { test } from 'node:test';

import { approveDecision, filterAccounts, initialQuery, manualCode, trLower } from './filter.ts';

const plan = [
  { code: '770', name: 'Genel Yönetim Giderleri', is_leaf: false },
  { code: '770.01', name: 'Kırtasiye Giderleri', is_leaf: true },
  { code: '770.02', name: 'İletişim Giderleri', is_leaf: true },
  { code: '770.10', name: 'Akaryakıt Giderleri', is_leaf: true },
  { code: '760.01', name: 'Pazarlama Kırtasiye', is_leaf: true },
  { code: '191.01', name: 'İndirilecek KDV %20', is_leaf: true },
  { code: '770.01', name: 'Kırtasiye Giderleri (tekrar)', is_leaf: true },
];

test('initialQuery opens the picker on the main account of the suggestion', () => {
  assert.equal(initialQuery('770.01.002'), '770');
  assert.equal(initialQuery(' 191 '), '191');
  assert.equal(initialQuery(''), '');
  assert.equal(initialQuery(null), '');
  assert.equal(initialQuery('ab'), '');
});

test('filterAccounts keeps leaf accounts, code prefix matches first, numeric order', () => {
  assert.deepEqual(
    filterAccounts(plan, '770').map((a) => a.code),
    ['770.01', '770.02', '770.10'],
  );
  // Ad araması Türkçe küçük harfle: "KIRTASİYE" → "kırtasiye".
  assert.deepEqual(
    filterAccounts(plan, 'KIRTASİYE').map((a) => a.code),
    ['760.01', '770.01'],
  );
  assert.deepEqual(
    filterAccounts(plan, 'iletişim').map((a) => a.code),
    ['770.02'],
  );
  assert.deepEqual(filterAccounts(plan, '   '), []);
  assert.equal(filterAccounts(plan, '7', 2).length, 2);
  assert.equal(trLower('İLETİŞİM'), 'iletişim');
});

test('manualCode offers a well-formed code that is not in the list', () => {
  const shown = filterAccounts(plan, '770');
  assert.equal(manualCode('770.99', shown), '770.99');
  assert.equal(manualCode('770.01', shown), null);
  assert.equal(manualCode('kırtasiye', shown), null);
  assert.equal(manualCode('77', shown), null);
});

test('approveDecision accepts as proposed or sends the chosen code', () => {
  assert.deepEqual(approveDecision('770.01', null), { action: 'accept' });
  assert.deepEqual(approveDecision('770.01', ' 770.01 '), { action: 'accept' });
  const d = approveDecision('770.01', '770.10');
  assert.equal(d.action, 'correct');
  assert.ok(d.action === 'correct' && d.accountCode === '770.10' && d.reason.length >= 3);
  // Hesap kodu önerisi değilse (fişten okunan alan) kod gönderilmez.
  assert.deepEqual(approveDecision(null, '770.10'), { action: 'accept' });
});

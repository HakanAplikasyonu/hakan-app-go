import assert from 'node:assert/strict';
import { test } from 'node:test';

import { bulkTargets, fieldLabel, groupQueue, proposedText, suggestionLabel, cardTilt, confidenceBand, confidencePercent, isValidAccountCode, otherPendingForTarget, proposedCode, queueEta, queueProgress, receiptApproveMessage, runSequential, swipeAction } from './format.ts';

test('proposedCode', () => {
  assert.equal(proposedCode({ account_code: '770.01' }), '770.01');
  assert.equal(proposedCode({ other: 1 }), null);
  assert.equal(proposedCode(null), null);
  assert.equal(proposedCode('770'), null);
});

test('account code format', () => {
  for (const ok of ['770', '770.01', '320.01.001', '153.01.A1']) assert.equal(isValidAccountCode(ok), true, ok);
  for (const bad of ['77', '770.', 'abc', '770..1', '']) assert.equal(isValidAccountCode(bad), false, bad);
});

test('confidence', () => {
  assert.equal(confidencePercent('0.8700'), '%87');
  assert.equal(confidencePercent('x'), '—');
  assert.equal(confidenceBand('0.5'), 'low');
  assert.equal(confidenceBand('0.7'), 'medium');
  assert.equal(confidenceBand('0.9'), 'high');
});

test('bulk targets: selected pending with engagement only', () => {
  const list = [
    { uuid: 'a', engagement_uuid: 'e', kind: 'k', target_field: 'f', proposed_value: {}, confidence: '1', status: 'pending' },
    { uuid: 'b', engagement_uuid: 'e', kind: 'k', target_field: 'f', proposed_value: {}, confidence: '1', status: 'accepted' },
    { uuid: 'c', kind: 'k', target_field: 'f', proposed_value: {}, confidence: '1', status: 'pending' },
    { uuid: 'd', engagement_uuid: 'e', kind: 'k', target_field: 'f', proposed_value: {}, confidence: '1', status: 'pending' },
  ];
  assert.deepEqual(bulkTargets(list, new Set(['a', 'b', 'c'])).map((s) => s.uuid), ['a']);
});

test('runSequential keeps going after a failure', async () => {
  const seen: number[] = [];
  const res = await runSequential([1, 2, 3], async (n) => {
    seen.push(n);
    if (n === 2) throw new Error('x');
  });
  assert.deepEqual(seen, [1, 2, 3]);
  assert.deepEqual(res.ok, [1, 3]);
  assert.equal(res.failed[0].item, 2);
});

test('swipe threshold', () => {
  assert.equal(swipeAction(200, 400), 'accept');
  assert.equal(swipeAction(-200, 400), 'reject');
  assert.equal(swipeAction(50, 400), null);
  assert.equal(swipeAction(70, 100), 'accept'); // minimum 60 px
});

test('swipe up corrects when vertical dominates', () => {
  assert.equal(swipeAction(10, 400, 0.35, -150, 430), 'correct');
  assert.equal(swipeAction(200, 400, 0.35, -150, 430), 'accept'); // yatay baskın
  assert.equal(swipeAction(0, 400, 0.35, -50, 430), null);
  assert.equal(swipeAction(0, 400, 0.35, -150), null); // yükseklik yok: düzeltme kapalı
});

test('card tilt, progress and eta', () => {
  assert.equal(cardTilt(0, 400), 0);
  assert.equal(cardTilt(400, 400), 8);
  assert.equal(cardTilt(-100, 400), -4);
  assert.equal(cardTilt(10, 0), 0);
  assert.equal(queueProgress(0, 48), 0);
  assert.equal(queueProgress(12, 36), 0.25);
  assert.equal(queueProgress(0, 0), 0);
  assert.equal(queueEta(48), '~5 dk');
  assert.equal(queueEta(5), "1 dk'dan az");
  assert.equal(queueEta(0), '');
});

test('groupQueue: mükellefe göre, ilk görünüş sırası', () => {
  const s = (uuid: string, e: string | undefined, c = '0.9') => ({ uuid, engagement_uuid: e, confidence: c });
  const list = [s('1', 'a'), s('2', 'b'), s('3', 'a'), s('4', undefined)];
  const g = groupQueue(list, 'mukellef', (e) => ({ a: 'Atlas', b: 'Kuzey' })[e]);
  assert.deepEqual(
    g.map((x) => [x.title, x.data.map((d) => d.uuid)]),
    [
      ['Atlas', ['1', '3']],
      ['Kuzey', ['2']],
      ['Mükellef', ['4']],
    ],
  );
});

test('groupQueue: güvene göre, önce düşük, boş grup yok', () => {
  const s = (uuid: string, c: string) => ({ uuid, engagement_uuid: 'a', confidence: c });
  const g = groupQueue([s('1', '0.95'), s('2', '0.40'), s('3', '0.90')], 'guven', () => 'x');
  assert.deepEqual(
    g.map((x) => [x.key, x.data.map((d) => d.uuid)]),
    [
      ['low', ['2']],
      ['high', ['1', '3']],
    ],
  );
});

test('alan adları Türkçe, ham ad gösterilmez (#375)', () => {
  assert.equal(fieldLabel('total_amount'), 'Fiş toplamı');
  assert.equal(fieldLabel('tax_amount'), 'KDV tutarı');
  assert.equal(fieldLabel('match'), 'Banka eşleşmesi');
  assert.equal(fieldLabel('yeni_alan'), 'Yeni alan');
});

test('önerilen değer: tutar, tarih, eşleşme puanı; JSON yok (#375)', () => {
  assert.equal(proposedText({ account_code: '770.01' }), '770.01');
  assert.equal(proposedText('1250.00'), '1.250,00');
  assert.equal(proposedText('2026-10-05'), '05.10.2026');
  assert.equal(proposedText('Z-0091'), 'Z-0091');
  const match = { bank_transaction_id: 1, score: 73, invoices: [{ id: 5, number: 'ATL2026000000102', parts: { amount: 40 } }] };
  assert.equal(proposedText(match), '%73');
  assert.ok(!proposedText({ a: { b: 1 } }).includes('{'));
  assert.equal(suggestionLabel({ kind: 'bank_match', target_field: 'match', proposed_value: match }), 'Banka eşleşmesi: Fatura ATL2026000000102');
  assert.equal(suggestionLabel({ kind: 'receipt_field', target_field: 'tax_amount', proposed_value: '208.33' }), 'KDV tutarı');
});

test('otherPendingForTarget finds the other open fields of the same receipt', () => {
  const s = { uuid: 'a', status: 'pending', target_type: 'expense', target_id: 7 };
  const list = [
    s,
    { uuid: 'b', status: 'pending', target_type: 'expense', target_id: 7 },
    { uuid: 'c', status: 'accepted', target_type: 'expense', target_id: 7 },
    { uuid: 'd', status: 'pending', target_type: 'expense', target_id: 8 },
    { uuid: 'e', status: 'pending', target_type: 'invoice_item', target_id: 7 },
  ];
  assert.deepEqual(
    otherPendingForTarget(list, s).map((x) => x.uuid),
    ['b'],
  );
  assert.deepEqual(otherPendingForTarget(list, { ...s, target_id: null }), []);
});

test('receiptApproveMessage tells whether the receipt was posted', () => {
  assert.equal(receiptApproveMessage('approved', '770.01'), 'Fiş onaylandı (770.01); yevmiye taslağı oluşturuldu.');
  assert.match(receiptApproveMessage('waiting', null), /diğer alanları/);
  assert.equal(receiptApproveMessage('skipped', '770'), 'Fiş alanı onaylandı.');
});

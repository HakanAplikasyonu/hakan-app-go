import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { test } from 'node:test';

import { parse } from 'yaml';

import { badge, canApproveRow, dayLabel, inboxTypes, rowSubtitle, typeHints, typeLabel, typeLabels } from './format.ts';

test('türler API sözleşmesindeki InboxItem.type ile aynı', () => {
  const spec = parse(readFileSync(new URL('../../../../backend/docs/openapi.yaml', import.meta.url), 'utf8'));
  const e = [...spec.components.schemas.InboxItem.properties.type.enum].sort();
  assert.deepEqual(Object.keys(typeLabels).sort(), e);
  assert.deepEqual(Object.keys(typeHints).sort(), e);
  assert.deepEqual([...inboxTypes].sort(), e);
});

test('altı tür Türkçe ve sıralı', () => {
  assert.equal(inboxTypes.length, 6);
  assert.deepEqual(inboxTypes.map(typeLabel), ['Fatura', 'Banka hareketi', 'Masraf fişi', 'Z raporu', 'Yabancı fatura', 'Geç belge']);
  assert.equal(typeLabel('bilinmeyen'), 'Kayıt');
});

test('tek dokunuş onay yalnız masraf ve Z, yetkiyle', () => {
  assert.equal(canApproveRow({ type: 'expense', can_approve: true }, true), true);
  assert.equal(canApproveRow({ type: 'retail_sale', can_approve: true }, true), true);
  assert.equal(canApproveRow({ type: 'expense', can_approve: true }, false), false);
  assert.equal(canApproveRow({ type: 'expense', can_approve: false }, true), false);
  assert.equal(canApproveRow({ type: 'invoice', can_approve: true }, true), false);
});

test('satır alt metni ve tarih', () => {
  const money = (a: string, c: string) => `${a} ${c}`;
  assert.equal(dayLabel('2026-11-05'), '05.11.2026');
  assert.equal(dayLabel(null), null);
  assert.equal(
    rowSubtitle(
      { type: 'expense', amount: '120.50', currency: 'TRY', date: '2026-11-05', description: 'Akaryakıt', reference: 'F-12', can_approve: true },
      money,
    ),
    '120.50 TRY · 05.11.2026 · Akaryakıt · F-12',
  );
  assert.equal(
    rowSubtitle({ type: 'late_document', amount: null, currency: 'TRY', date: null, description: '', reference: null, can_approve: false }, money),
    '',
  );
});

test('rozet', () => {
  assert.equal(badge(0), null);
  assert.equal(badge(undefined), null);
  assert.equal(badge(7), '7');
  assert.equal(badge(150), '99+');
});

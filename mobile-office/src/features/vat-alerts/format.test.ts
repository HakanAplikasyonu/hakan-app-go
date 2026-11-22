import assert from 'node:assert/strict';
import { test } from 'node:test';

import { dateLabel, eventLabel, eventTone, payableOf, recentCount } from './format.ts';

test('olay türleri Türkçe', () => {
  assert.equal(eventLabel('vat.threshold_exceeded'), 'KDV eşiği aşıldı');
  assert.equal(eventLabel('vat.status_changed'), 'KDV durumu değişti');
  assert.equal(eventLabel('vat.report_ready'), 'KDV raporu hazır');
  assert.equal(eventLabel('vat.unknown'), 'KDV uyarısı');
  assert.equal(eventTone('vat.threshold_exceeded'), 'danger');
});

test('ödenecek KDV payload içinden', () => {
  assert.equal(payableOf({ payable: '1250.50' }), '1250.50');
  assert.equal(payableOf({ payable: 300 }), '300');
  assert.equal(payableOf({ payable: '' }), null);
  assert.equal(payableOf({}), null);
  assert.equal(payableOf(undefined), null);
});

test('tarih ve son 7 gün sayısı', () => {
  assert.equal(dateLabel('bozuk'), 'bozuk');
  assert.match(dateLabel('2026-11-22T09:30:00Z'), /^\d{2}\.11\.2026 \d{2}:\d{2}$/);
  const now = new Date('2026-11-22T12:00:00Z');
  const items = [{ created_at: '2026-11-21T12:00:00Z' }, { created_at: '2026-11-10T12:00:00Z' }, { created_at: 'x' }];
  assert.equal(recentCount(items, now), 1);
});

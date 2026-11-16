import assert from 'node:assert/strict';
import { test } from 'node:test';

import { applyDecision, canDecide, periodLabel, sortWarnings, warningMessage, type DataWarning } from './format.ts';

const w = (uuid: string, severity: DataWarning['severity'], updated_at: string, extra: Partial<DataWarning> = {}): DataWarning => ({
  uuid,
  engagement_uuid: 'e1',
  period: '2026-10',
  entity_type: 'invoice',
  entity_uuid: null,
  code: 'TAX_TOTAL_MISMATCH',
  severity,
  params: {},
  producer: 'control:test',
  status: 'open',
  resolved_at: null,
  created_at: updated_at,
  updated_at,
  ...extra,
});

test('warningMessage fills params and falls back for unknown codes', () => {
  assert.equal(
    warningMessage('TAX_TOTAL_MISMATCH', { actual: '18,00', expected: '20,00' }),
    "Satır KDV'lerinin toplamı (18,00) başlıktaki KDV'den (20,00) farklı.",
  );
  assert.match(warningMessage('TAX_TOTAL_MISMATCH', {}), /\(—\)/);
  assert.equal(warningMessage('NOPE_CODE'), 'Kontrol uyarısı (NOPE_CODE)');
});

test('periodLabel', () => {
  assert.equal(periodLabel('2026-10'), 'Ekim 2026');
  assert.equal(periodLabel(null), null);
  assert.equal(periodLabel('x'), 'x');
});

test('sort: severity then newest', () => {
  const list = [w('a', 'info', '2026-11-03'), w('b', 'blocker', '2026-11-01'), w('c', 'warning', '2026-11-02'), w('d', 'blocker', '2026-11-05')];
  assert.deepEqual(sortWarnings(list).map((x) => x.uuid), ['d', 'b', 'c', 'a']);
});

test('decisions only on open warnings, applied optimistically', () => {
  assert.equal(canDecide({ status: 'open' }), true);
  assert.equal(canDecide({ status: 'resolved' }), false);
  const out = applyDecision([w('a', 'info', '2026-11-01'), w('b', 'info', '2026-11-01')], 'b', 'resolved', '2026-11-16T10:00:00Z');
  assert.equal(out[0].status, 'open');
  assert.equal(out[1].status, 'resolved');
  assert.equal(out[1].resolved_at, '2026-11-16T10:00:00Z');
});

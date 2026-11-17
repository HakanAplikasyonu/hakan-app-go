import assert from 'node:assert/strict';
import { test } from 'node:test';

import { archiveFilters, documentSubtitle, expiryChip, expiryWarning } from './format.ts';

test('expiryChip', () => {
  assert.equal(expiryChip({ expiry_status: 'none', days_left: null, valid_until: null }), null);
  assert.deepEqual(expiryChip({ expiry_status: 'expired', days_left: -5, valid_until: '2026-11-12' }), { label: 'Süresi doldu (5 gün)', tone: 'danger' });
  assert.deepEqual(expiryChip({ expiry_status: 'expired', days_left: -1, valid_until: '2026-11-16' }), { label: 'Dün doldu', tone: 'danger' });
  assert.deepEqual(expiryChip({ expiry_status: 'expiring', days_left: 0, valid_until: '2026-11-17' }), { label: 'Bugün bitiyor', tone: 'warning' });
  assert.deepEqual(expiryChip({ expiry_status: 'expiring', days_left: 20, valid_until: '2026-12-07' }), { label: '20 gün kaldı', tone: 'warning' });
  assert.deepEqual(expiryChip({ expiry_status: 'valid', days_left: 400, valid_until: '2027-12-31' }), { label: '31.12.2027 tarihine kadar', tone: 'success' });
});

test('documentSubtitle', () => {
  assert.equal(
    documentSubtitle({ category: 'isg', isg_kind: 'health_report', document_date: '2021-11-12', valid_until: '2026-11-12' }),
    'Sağlık raporu · 12.11.2021 · bitiş 12.11.2026',
  );
  assert.equal(documentSubtitle({ category: 'trade_registry', isg_kind: null, document_date: null, valid_until: null }), 'Ticaret sicil');
});

test('expiryWarning', () => {
  assert.equal(expiryWarning(0, 0, 30), null);
  assert.equal(expiryWarning(1, 2, 30), '1 İSG evrakının süresi doldu, 2 İSG evrakı 30 gün içinde bitiyor.');
  assert.equal(expiryWarning(0, 1, 30), '1 İSG evrakı 30 gün içinde bitiyor.');
});

test('archive filters start with Tümü and include every category', () => {
  assert.equal(archiveFilters[0].key, 'all');
  assert.equal(archiveFilters.length, 8);
});

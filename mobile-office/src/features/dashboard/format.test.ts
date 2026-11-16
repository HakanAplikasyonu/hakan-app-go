import assert from 'node:assert/strict';
import { test } from 'node:test';

import { clock, countLabel, daysUntilVatDue, declarationPeriod, dueText, greeting, periodCrown } from './format.ts';

test('clock shows Istanbul time', () => {
  assert.equal(clock('2026-11-16T11:05:00Z'), '14:05');
  assert.equal(clock('nope'), '');
});

test('countLabel', () => {
  assert.equal(countLabel(0), 'Yok');
  assert.equal(countLabel(undefined), 'Yok');
  assert.equal(countLabel(12), '12');
  assert.equal(countLabel(1200), '999+');
});

test('greeting by Istanbul hour', () => {
  assert.equal(greeting(new Date('2026-11-16T06:00:00Z')), 'Günaydın'); // 09:00
  assert.equal(greeting(new Date('2026-11-16T12:00:00Z')), 'İyi günler'); // 15:00
  assert.equal(greeting(new Date('2026-11-16T18:00:00Z')), 'İyi akşamlar'); // 21:00
  assert.equal(greeting(new Date('2026-11-16T00:00:00Z')), 'İyi geceler'); // 03:00
});

test('declaration period and VAT due day', () => {
  assert.equal(declarationPeriod(new Date('2026-11-16T09:00:00Z')), '2026-10');
  assert.equal(declarationPeriod(new Date('2026-01-05T09:00:00Z')), '2025-12');
  assert.equal(declarationPeriod(new Date('2026-10-31T22:30:00Z')), '2026-10'); // İstanbul'da 1 Kasım
  assert.equal(daysUntilVatDue(new Date('2026-11-16T09:00:00Z')), 12);
  assert.equal(daysUntilVatDue(new Date('2026-11-29T09:00:00Z')), 0);
  assert.equal(dueText(12), '12 gün kaldı');
  assert.equal(dueText(0), 'bugün son gün');
});

test('period crown counts ready cells of the period', () => {
  const rows = [
    { cells: [{ period: '2026-10', ready: true }] },
    { cells: [{ period: '2026-10', ready: false }] },
    { cells: [{ period: '2026-09', ready: true }] },
  ];
  assert.deepEqual(periodCrown(rows, '2026-10'), { ready: 1, total: 2 });
  assert.deepEqual(periodCrown([], '2026-10'), { ready: 0, total: 0 });
});

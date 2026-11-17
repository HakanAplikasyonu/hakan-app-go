import assert from 'node:assert/strict';
import { test } from 'node:test';

import { groupByPeriod, periodLabel } from './group.ts';

test('periodLabel Türkçe ay adı', () => {
  assert.equal(periodLabel('2026-10'), 'Ekim 2026');
  assert.equal(periodLabel('2026-13'), '2026-13');
});

test('groupByPeriod sırayı korur', () => {
  const docs = [
    { uuid: 'a', period: '2026-10' },
    { uuid: 'b', period: '2026-10' },
    { uuid: 'c', period: '2026-09' },
  ];
  const g = groupByPeriod(docs);
  assert.deepEqual(
    g.map((x) => [x.period, x.items.map((d) => d.uuid)]),
    [
      ['2026-10', ['a', 'b']],
      ['2026-09', ['c']],
    ],
  );
});

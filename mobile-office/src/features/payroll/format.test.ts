import assert from 'node:assert/strict';
import { test } from 'node:test';

import { groupByEmployee, periodsOf } from './format.ts';

const doc = (uuid: string, period: string, employee: [string, string] | null, title = uuid) => ({
  uuid,
  period,
  kind: employee ? ('payslip' as const) : ('payroll' as const),
  title,
  employee_uuid: employee?.[0] ?? null,
  employee_name: employee?.[1] ?? null,
});

test('groupByEmployee puts company-wide documents first, employees by Turkish name order', () => {
  const groups = groupByEmployee([
    doc('a', '2026-09', ['e2', 'Şule Kaya']),
    doc('b', '2026-10', null),
    doc('c', '2026-10', ['e1', 'Can Demir']),
    doc('d', '2026-10', ['e2', 'Şule Kaya']),
    doc('e', '2026-09', null),
  ]);
  assert.deepEqual(
    groups.map((g) => [g.title, g.items.map((i) => i.uuid)]),
    [
      ['Şirket geneli', ['b', 'e']],
      ['Can Demir', ['c']],
      ['Şule Kaya', ['d', 'a']],
    ],
  );
  assert.deepEqual(groupByEmployee([]), []);
});

test('periodsOf merges and sorts newest first', () => {
  assert.deepEqual(periodsOf([{ period: '2026-09' }, { period: '2026-10' }], [{ period: '2026-10' }, { period: '2026-08' }]), [
    '2026-10',
    '2026-09',
    '2026-08',
  ]);
});

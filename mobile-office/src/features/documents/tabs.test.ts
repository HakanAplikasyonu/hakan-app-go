import assert from 'node:assert/strict';
import { test } from 'node:test';

import { visibleTabs } from './tabs.ts';

test('visibleTabs hides tabs without permission and falls back to the first', () => {
  const all = visibleTabs(() => true, 'nope');
  assert.equal(all.selected, all.tabs[0].key);
  const none = visibleTabs(() => false, 'payroll');
  assert.deepEqual(none.tabs, []);
  assert.equal(none.selected, undefined);
  assert.equal(visibleTabs((p) => p === 'tenant.payroll.read', 'payroll').selected, 'payroll');
});

import assert from 'node:assert/strict';
import { test } from 'node:test';

import { contactRows } from './contact.ts';

test('contactRows shows the office contact details', () => {
  assert.deepEqual(contactRows({ phone: '0532 123 45 67', email: 'muhasebe@ornek.test' }), [
    { key: 'phone', label: 'Telefon', value: '0532 123 45 67', set: true },
    { key: 'email', label: 'E-posta', value: 'muhasebe@ornek.test', set: true },
  ]);
});

test('contactRows marks missing values', () => {
  for (const c of [null, undefined, { phone: null, email: '  ' }]) {
    const rows = contactRows(c);
    assert.equal(rows.length, 2);
    assert.ok(rows.every((r) => !r.set && r.value === 'Kayıtlı değil'));
  }
});

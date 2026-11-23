import assert from 'node:assert/strict';
import { test } from 'node:test';

import { applyReading, emptyForm, normalizeTaxId, requestBody, taxIdKind, validateNewTaxpayer } from './certificate.ts';

test('tax id kind', () => {
  assert.equal(normalizeTaxId(' 123.456-7890 '), '1234567890');
  assert.equal(taxIdKind('1234567890'), 'legal');
  assert.equal(taxIdKind('123 456 789 01'), 'individual');
  assert.equal(taxIdKind('12345'), null);
  assert.equal(taxIdKind('12345678ab'), null);
});

test('applyReading fills read fields and marks the rest', () => {
  const { form, unread } = applyReading(
    { ...emptyForm, contact_email: 'a@b.co', address: 'eski' },
    { tax_id: '123 456 7890', title: ' ÖRNEK LTD ŞTİ ', tax_office: 'Kadıköy', address: null, activity_code: '' },
  );
  assert.equal(form.tax_id, '1234567890');
  assert.equal(form.title, 'ÖRNEK LTD ŞTİ');
  assert.equal(form.tax_office, 'Kadıköy');
  assert.equal(form.address, '');
  assert.equal(form.contact_email, 'a@b.co');
  assert.deepEqual(unread, ['address', 'activity_code']);
});

test('applyReading with nothing read', () => {
  const { unread } = applyReading(emptyForm, {});
  assert.deepEqual(unread, ['tax_id', 'title', 'tax_office', 'address', 'activity_code']);
});

test('validateNewTaxpayer', () => {
  assert.deepEqual(validateNewTaxpayer({ ...emptyForm, tax_id: '1234567890', title: 'X', contact_email: 'a@b.co' }), {});
  const e = validateNewTaxpayer(emptyForm);
  assert.ok(e.tax_id && e.title && e.contact_email);
});

test('requestBody', () => {
  assert.deepEqual(requestBody({ ...emptyForm, tax_id: '123 456 789 01', title: ' Ali ', contact_email: ' a@b.co ' }), {
    kind: 'individual',
    tax_id: '12345678901',
    title: 'Ali',
    contact_email: 'a@b.co',
  });
});

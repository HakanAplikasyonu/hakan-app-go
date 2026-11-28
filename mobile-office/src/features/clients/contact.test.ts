import assert from 'node:assert/strict';
import { test } from 'node:test';

import { contactBody, contactFieldErrors, contactPerson } from './contact.ts';

test('contactPerson lists owners only', () => {
  assert.equal(
    contactPerson([
      { name: 'Ayşe', surname: 'Demir', email: 'ayse@ornek.test', role: 'owner' },
      { name: 'Can', surname: 'Er', email: 'can@ornek.test', role: 'employee' },
      { name: 'Ali', surname: 'Kaya', email: 'ali@ornek.test', role: 'owner' },
    ]),
    'Ayşe Demir, Ali Kaya',
  );
  assert.equal(contactPerson([{ name: 'Can', surname: 'Er', email: '', role: 'employee' }]), null);
  assert.equal(contactPerson(undefined), null);
});

test('contactFieldErrors maps server codes', () => {
  assert.deepEqual(contactFieldErrors([{ field: 'phone', code: 'telefon_gecersiz' }]), {
    phone: 'Telefon numarası geçersiz. Örnek: 0532 123 45 67',
  });
  const both = contactFieldErrors([{ code: 'telefon_gecersiz' }, { code: 'eposta_gecersiz' }]);
  assert.ok(both.phone && both.email);
  assert.deepEqual(contactFieldErrors([{ code: 'other' }]), {});
  assert.deepEqual(contactFieldErrors(undefined), {});
});

test('contactFieldErrors maps authorized person fields', () => {
  const errs = contactFieldErrors([
    { field: 'person_name', code: 'ad_gecersiz' },
    { field: 'person_phone', code: 'telefon_gecersiz' },
    { field: 'person_email', code: 'eposta_gecersiz' },
  ]);
  assert.equal(errs.person_name, 'Ad en fazla 120 karakter olabilir.');
  assert.ok(errs.person_phone && errs.person_email);
  assert.equal(errs.phone, undefined);
  assert.equal(errs.email, undefined);
});

test('contactBody trims', () => {
  assert.deepEqual(
    contactBody({ phone: ' 0532 123 45 67 ', email: '  ', person_name: '  Deniz Örnek ', person_phone: '', person_email: ' d@ornek.test' }),
    { phone: '0532 123 45 67', email: '', person_name: 'Deniz Örnek', person_phone: '', person_email: 'd@ornek.test' },
  );
});

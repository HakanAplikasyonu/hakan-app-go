import assert from 'node:assert/strict';
import { test } from 'node:test';

import { ApiError, createErrorMessage, isApiError, unwrap } from './envelope.ts';

test('unwrap returns data of a success envelope', () => {
  assert.deepEqual(unwrap(200, { success: true, data: { a: 1 }, meta: {} }), { a: 1 });
  assert.equal(unwrap(204, undefined), undefined);
});

test('unwrap throws the API error with code, details and request id', () => {
  assert.throws(
    () => unwrap(422, { success: false, error: { code: 'VALIDATION_ERROR', message: 'bad', details: [{ field: 'email', message: 'E-posta geçersiz' }] } }, 'req-1'),
    (e: unknown) =>
      isApiError(e) && e.status === 422 && e.code === 'VALIDATION_ERROR' && e.details[0].field === 'email' && e.requestId === 'req-1',
  );
});

test('unwrap: a 2xx with success false is an error; no body names the status', () => {
  assert.throws(() => unwrap(200, { success: false, error: { code: 'X', message: 'm' } }), (e: unknown) => isApiError(e) && e.code === 'X');
  assert.throws(() => unwrap(502, undefined), (e: unknown) => e instanceof ApiError && e.code === 'HTTP_502' && e.message === 'Request failed (502)');
  assert.throws(() => unwrap(0, null), (e: unknown) => e instanceof ApiError && e.code === 'NETWORK_ERROR');
});

test('createErrorMessage: common codes, app codes, validation detail, server fallback', () => {
  const msg = createErrorMessage({ NO_TENANT_MEMBERSHIP: 'Bu ofise erişiminiz yok.', RATE_LIMITED: 'uygulamanın metni' });
  const err = (code: string, message = 'sunucu', details?: { message?: string }[]) => new ApiError({ status: 400, code, message, details });
  assert.equal(msg(err('INVALID_CREDENTIALS')), 'E-posta veya parola hatalı.');
  assert.equal(msg(err('TIMEOUT')), 'Sunucuya ulaşılamadı. Bağlantınızı kontrol edin.');
  assert.equal(msg(err('NO_TENANT_MEMBERSHIP')), 'Bu ofise erişiminiz yok.');
  assert.equal(msg(err('RATE_LIMITED')), 'uygulamanın metni', 'app text wins over the common one');
  assert.equal(msg(err('VALIDATION_ERROR', 'x', [{ message: 'Tutar gerekli' }])), 'Tutar gerekli');
  assert.equal(msg(err('VALIDATION_ERROR', 'x')), 'Girilen bilgileri kontrol edin.');
  assert.equal(msg(err('CONFLICT', 'Zaten var')), 'Zaten var');
  assert.equal(msg(err('CONFLICT', '')), 'Beklenmeyen bir hata oluştu.');
  assert.equal(msg(err('constructor', 'sunucu')), 'sunucu', 'prototype keys are not messages');
  assert.equal(msg(new Error('boom')), 'Beklenmeyen bir hata oluştu.');
  assert.equal(createErrorMessage()(err('NO_TENANT_MEMBERSHIP', 'srv')), 'srv');
});

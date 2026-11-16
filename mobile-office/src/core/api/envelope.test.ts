// Kaynak: mobile/src/core/api/envelope.test.ts (#236'da kopyalandı; ortak paket ayrı issue).
import assert from 'node:assert/strict';
import { test } from 'node:test';

import { ApiError, errorMessage, unwrap } from './envelope.ts';

test('unwrap returns data of a success envelope', () => {
  assert.deepEqual(unwrap(200, { success: true, data: { a: 1 }, meta: {} }), { a: 1 });
});

test('unwrap throws the API error code', () => {
  assert.throws(
    () => unwrap(401, { success: false, error: { code: 'INVALID_CREDENTIALS', message: 'no' }, meta: {} }, 'req-1'),
    (e: unknown) => e instanceof ApiError && e.code === 'INVALID_CREDENTIALS' && e.status === 401 && e.requestId === 'req-1',
  );
});

test('unwrap without a body names the HTTP status', () => {
  assert.throws(() => unwrap(502, undefined), (e: unknown) => e instanceof ApiError && e.code === 'HTTP_502');
});

test('errorMessage is Turkish and falls back to the server message', () => {
  assert.equal(errorMessage(new ApiError({ status: 401, code: 'INVALID_CREDENTIALS', message: 'x' })), 'E-posta veya parola hatalı.');
  assert.equal(errorMessage(new ApiError({ status: 409, code: 'CONFLICT', message: 'Zaten var' })), 'Zaten var');
  assert.equal(errorMessage(new Error('boom')), 'Beklenmeyen bir hata oluştu.');
});

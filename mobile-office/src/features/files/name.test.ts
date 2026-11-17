import assert from 'node:assert/strict';
import { test } from 'node:test';

import { cacheFileName, fileSize } from './name.ts';

test('cacheFileName keeps the extension and strips unsafe characters', () => {
  assert.equal(cacheFileName('bordro', 'ab-12', 'Ekim 2026.PDF'), 'bordro-ab-12.pdf');
  assert.equal(cacheFileName('evrak', '../x', null, 'image/jpeg'), 'evrak-x.jpg');
  assert.equal(cacheFileName('evrak', 'u', 'adsiz', 'application/octet-stream'), 'evrak-u');
});

test('fileSize in tr-TR', () => {
  assert.equal(fileSize(512), '512 B');
  assert.equal(fileSize(1536), '1,5 KB');
  assert.equal(fileSize(5 * 1024 * 1024), '5 MB');
  assert.equal(fileSize(-1), '');
});

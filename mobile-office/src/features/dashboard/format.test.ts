import assert from 'node:assert/strict';
import { test } from 'node:test';

import { clock, countLabel, greeting } from './format.ts';

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

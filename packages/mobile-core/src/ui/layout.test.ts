import assert from 'node:assert/strict';
import { test } from 'node:test';

import { rayMi, rozet, yerlesim } from './layout.ts';

test('breakpoints', () => {
  assert.equal(yerlesim(390), 'telefon');
  assert.equal(yerlesim(767), 'telefon');
  assert.equal(yerlesim(768), 'tablet');
  assert.equal(yerlesim(1194), 'tablet');
  assert.equal(yerlesim(1280), 'genis');
  assert.equal(rayMi(390), false);
  assert.equal(rayMi(1024), true);
});

test('badge text', () => {
  assert.equal(rozet(0), null);
  assert.equal(rozet(undefined), null);
  assert.equal(rozet(7), '7');
  assert.equal(rozet(120), '99+');
});

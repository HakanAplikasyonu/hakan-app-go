import assert from 'node:assert/strict';
import { test } from 'node:test';

import { safeLink } from '../config/safe-link.ts';
import { launchFailure } from './launch.ts';

test('only a refused session signs out at launch', () => {
  assert.equal(launchFailure({ status: 401, code: 'UNAUTHORIZED' }), 'signedOut');
  assert.equal(launchFailure({ status: 0, code: 'NETWORK_ERROR' }), 'offline');
  assert.equal(launchFailure({ status: 0, code: 'TIMEOUT' }), 'offline');
  assert.equal(launchFailure({ status: 503 }), 'offline');
  assert.equal(launchFailure(new TypeError('Network request failed')), 'offline');
  assert.equal(launchFailure(undefined), 'offline');
});

test('profile links accept https (and mailto for support) only', () => {
  assert.equal(safeLink(' https://example.com/gizlilik '), 'https://example.com/gizlilik');
  assert.equal(safeLink('http://example.com'), '');
  assert.equal(safeLink('javascript:alert(1)'), '');
  assert.equal(safeLink('mailto:destek@example.com'), '');
  assert.equal(safeLink('mailto:destek@example.com', true), 'mailto:destek@example.com');
  assert.equal(safeLink(undefined), '');
});

import assert from 'node:assert/strict';
import { test } from 'node:test';

import { safeLink } from '../config/safe-link.ts';
import { launchFailure } from './launch.ts';

test('only a refused session signs out at launch', () => {
  assert.equal(launchFailure({ status: 401, code: 'UNAUTHORIZED' }), 'signedOut');
  assert.equal(launchFailure({ status: 0, code: 'NETWORK_ERROR' }), 'offline');
  assert.equal(launchFailure({ status: 503 }), 'offline');
  assert.equal(launchFailure(undefined), 'offline');
});

test('safeLink accepts https only', () => {
  assert.equal(safeLink(' https://o123.ingest.sentry.io/1 '), 'https://o123.ingest.sentry.io/1');
  assert.equal(safeLink('http://example.com'), '');
  assert.equal(safeLink(undefined), '');
});

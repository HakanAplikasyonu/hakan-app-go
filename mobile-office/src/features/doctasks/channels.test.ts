import assert from 'node:assert/strict';
import { test } from 'node:test';

import { canRemind, channelsText, extraChannels, isComingSoon, toggleChannel } from './channels.ts';

test('toggleChannel keeps push and a fixed order', () => {
  assert.deepEqual(toggleChannel(['push'], 'email'), ['push', 'email']);
  assert.deepEqual(toggleChannel(['push', 'email'], 'sms'), ['push', 'sms', 'email']);
  assert.deepEqual(toggleChannel(['push', 'sms', 'email'], 'sms'), ['push', 'email']);
  assert.deepEqual(toggleChannel(['push', 'sms'], 'push'), ['push', 'sms']);
});

test('extraChannels drops push', () => {
  assert.deepEqual(extraChannels(['push']), []);
  assert.deepEqual(extraChannels(['email', 'push', 'sms']), ['sms', 'email']);
});

test('channelsText', () => {
  assert.equal(channelsText([]), '');
  assert.equal(channelsText(['sms']), 'SMS');
  assert.equal(channelsText(['email', 'sms']), 'SMS ve e-posta');
  assert.equal(channelsText(['push', 'sms', 'email']), 'uygulama bildirimi, SMS ve e-posta');
});

test('canRemind only for open requests', () => {
  assert.equal(canRemind('open'), true);
  assert.equal(canRemind('submitted'), false);
  assert.equal(canRemind('matched'), false);
});

test('isComingSoon', () => {
  assert.equal(isComingSoon({ status: 404 }), true);
  assert.equal(isComingSoon({ status: 405 }), true);
  assert.equal(isComingSoon({ status: 400 }), false);
  assert.equal(isComingSoon(new Error('x')), false);
  assert.equal(isComingSoon(null), false);
});

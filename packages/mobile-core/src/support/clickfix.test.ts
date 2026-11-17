import assert from 'node:assert/strict';
import { test } from 'node:test';

import {
  DEFAULT_CLICKFIX_EMBED_URL,
  bridgeScript,
  cleanEmbedUrl,
  cleanWidgetId,
  embedOrigin,
  embedPageUrl,
  externalLink,
  forgetOtherUserScript,
  isAllowedNavigation,
  parseBridgeMessage,
  visitorCommands,
} from './clickfix.ts';

test('widget id: empty or malformed = no support screen', () => {
  assert.equal(cleanWidgetId(''), null);
  assert.equal(cleanWidgetId(undefined), null);
  assert.equal(cleanWidgetId('a b'), null);
  assert.equal(cleanWidgetId(' ofis-1 '), 'ofis-1');
});

test('embed page: https only, widget and locale in the query', () => {
  assert.equal(cleanEmbedUrl('http://x.example/e.html'), DEFAULT_CLICKFIX_EMBED_URL);
  assert.equal(cleanEmbedUrl('javascript:alert(1)'), DEFAULT_CLICKFIX_EMBED_URL);
  const u = new URL(embedPageUrl('', 'w-1', { locale: 'tr-TR' }));
  assert.equal(u.origin, 'https://getclickfix.com');
  assert.equal(u.searchParams.get('widget'), 'w-1');
  assert.equal(u.searchParams.get('locale'), 'tr');
  assert.equal(embedOrigin('https://destek.example.com/widget-embed.html'), 'https://destek.example.com');
});

test('navigation stays on the embed origin; links are https only', () => {
  const o = 'https://getclickfix.com';
  assert.equal(isAllowedNavigation('https://getclickfix.com/widget-embed.html?widget=x', o), true);
  assert.equal(isAllowedNavigation('https://evil.example/', o), false);
  assert.equal(isAllowedNavigation('intent://x', o), false);
  assert.equal(externalLink('https://ornek.com/a'), 'https://ornek.com/a');
  assert.equal(externalLink('tel:123'), null);
  assert.equal(externalLink(3), null);
});

test('bridge messages', () => {
  assert.deepEqual(parseBridgeMessage('{"type":"clickfix:ready"}'), { type: 'ready' });
  assert.deepEqual(parseBridgeMessage('{"type":"clickfix:event","event":"close"}'), { type: 'close' });
  assert.deepEqual(parseBridgeMessage('{"type":"clickfix:link","url":"https://a.example/x"}'), {
    type: 'link',
    url: 'https://a.example/x',
  });
  assert.deepEqual(parseBridgeMessage('{"type":"clickfix:link","url":"file:///etc"}'), { type: 'other' });
  assert.equal(parseBridgeMessage('not json'), null);
  assert.equal(parseBridgeMessage('{"type":"x"}'), null);
});

test('visitor commands: identify when signed, update otherwise', () => {
  const v = { name: 'Ada', email: 'ada@example.com', attributes: { ofis: 'Demir', bos: '' } };
  const signed = visitorCommands(v, { user_id: 'u', user_hash: 'h', company: { id: 'c' } });
  assert.equal((signed[0] as { type: string }).type, 'clickfix:identify');
  assert.deepEqual((signed[0] as { user: unknown }).user, {
    user_id: 'u',
    user_hash: 'h',
    company: { id: 'c' },
    name: 'Ada',
    email: 'ada@example.com',
    attributes: { ofis: 'Demir' },
  });
  assert.deepEqual(visitorCommands(v, null), [
    { type: 'clickfix:update', update: { ofis: 'Demir', name: 'Ada', email: 'ada@example.com' } },
  ]);
  assert.deepEqual(visitorCommands({}, null), []);
  assert.match(bridgeScript({ type: 'clickfix:reset' }), /ClickFixBridge\.receive\("\{\\"type\\":\\"clickfix:reset\\"\}"\)/);
});

test('forgetOtherUserScript embeds the user key as a JSON string', () => {
  const js = forgetOtherUserScript('u-1"x');
  assert.match(js, /localStorage\.getItem\(k\)!=="u-1\\"x"/);
  assert.ok(js.endsWith('true;'));
});

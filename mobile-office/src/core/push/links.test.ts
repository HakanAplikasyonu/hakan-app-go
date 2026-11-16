import assert from 'node:assert/strict';
import { test } from 'node:test';

import { hrefOf, notificationUrl, resolveLink, systemPath } from './links.ts';

const ENG = '0b6a5d1e-2f3c-4a5b-8c7d-9e0f1a2b3c4d';
const TASK = 'aa11bb22-cc33-4d44-8e55-ff6677889900';

test('office panel paths map to app routes with their office', () => {
  assert.deepEqual(resolveLink('/t/demir-smmm'), { route: '/', office: 'demir-smmm' });
  assert.deepEqual(resolveLink(`/t/demir-smmm/engagements/${ENG}?task=${TASK}`), {
    route: '/tasks/[uuid]',
    params: { uuid: TASK, engagement: ENG },
    office: 'demir-smmm',
  });
  assert.deepEqual(resolveLink(`/t/demir-smmm/engagements/${ENG}`), {
    route: '/clients/[engagement]',
    params: { engagement: ENG },
    office: 'demir-smmm',
  });
  assert.deepEqual(resolveLink(`/t/demir-smmm/warnings?engagement=${ENG}`), {
    route: '/warnings',
    params: { engagement: ENG },
    office: 'demir-smmm',
  });
  assert.deepEqual(resolveLink('/t/demir-smmm/review'), { route: '/review', office: 'demir-smmm' });
});

test('universal links use the same paths; uuids are lower-cased', () => {
  assert.deepEqual(resolveLink(`https://app.example.com/t/Ofis/engagements/${ENG.toUpperCase()}#x`), {
    route: '/clients/[engagement]',
    params: { engagement: ENG },
    office: 'ofis',
  });
});

test('hakanofis scheme links', () => {
  assert.deepEqual(resolveLink('hakanofis://warnings'), { route: '/warnings' });
  assert.deepEqual(resolveLink('hakanofis://review'), { route: '/review' });
  assert.deepEqual(resolveLink(`hakanofis://clients/${ENG}`), { route: '/clients/[engagement]', params: { engagement: ENG } });
  assert.deepEqual(resolveLink(`hakanofis://tasks/${TASK}?engagement=${ENG}`), {
    route: '/tasks/[uuid]',
    params: { uuid: TASK, engagement: ENG },
  });
  assert.deepEqual(resolveLink('hakanofis://'), { route: '/' });
});

test('unknown or malformed links are dropped', () => {
  assert.equal(resolveLink('/t/demir-smmm/billing'), null);
  assert.equal(resolveLink('/t/demir-smmm/engagements/not-a-uuid'), null);
  assert.equal(resolveLink(`hakanofis://tasks/${TASK}`), null); // engagement gerekli
  assert.equal(resolveLink('/m/x/tasks'), null); // mükellef paneli
  assert.equal(resolveLink('/t/../warnings'), null);
  assert.equal(resolveLink(''), null);
  assert.equal(resolveLink(null), null);
});

test('hrefOf and systemPath', () => {
  assert.equal(hrefOf({ route: '/tasks/[uuid]', params: { uuid: TASK, engagement: ENG } }), `/tasks/${TASK}?engagement=${ENG}`);
  assert.equal(hrefOf({ route: '/warnings', params: { engagement: ENG } }), `/warnings?engagement=${ENG}`);
  assert.equal(systemPath(`/t/x/engagements/${ENG}`), `/clients/${ENG}`);
  assert.equal(systemPath('/nope'), '/');
  assert.equal(notificationUrl({ url: '/t/x/review' }), '/t/x/review');
  assert.equal(notificationUrl({ url: 3 }), null);
});

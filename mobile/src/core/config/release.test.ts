// Sürüm hazırlığı denetimleri (#183): eas.json profilleri, uygulama kimliği,
// sürüm numarası stratejisi ve mağaza meta veri taslakları tutarlı kalmalı.
// Mağaza gönderimi yapılmaz; bu test yalnızca yapılandırmayı denetler.
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { test } from 'node:test';

const read = (p: string) => JSON.parse(readFileSync(new URL(`../../../${p}`, import.meta.url), 'utf8'));

const app = read('app.json').expo;
const pkg = read('package.json');
const eas = read('eas.json');
const store = read('store/store.config.json');

test('user-facing version is semver and matches package.json', () => {
  assert.match(app.version, /^\d+\.\d+\.\d+$/);
  assert.equal(app.version, pkg.version);
});

test('build numbers come from EAS (remote) and production increments them', () => {
  assert.equal(eas.cli.appVersionSource, 'remote');
  assert.equal(eas.build.production.autoIncrement, true);
  assert.equal(eas.cli.requireCommit, true);
});

test('one app id on both platforms, no change without a decision', () => {
  assert.equal(app.ios.bundleIdentifier, 'app.hakan.mukellef');
  assert.equal(app.android.package, 'app.hakan.mukellef');
  assert.equal(app.scheme, 'hakan');
});

test('profiles: development / preview are internal, production goes to the stores', () => {
  const { development, preview, production } = eas.build;
  for (const [name, p] of Object.entries({ development, preview, production })) {
    assert.equal(p.env.EXPO_PUBLIC_APP_ENV, name, `${name} env`);
  }
  assert.equal(development.developmentClient, true);
  assert.equal(development.distribution, 'internal');
  assert.equal(preview.distribution, 'internal');
  assert.equal(production.distribution, 'store');
  assert.equal(production.android.buildType, 'app-bundle');
  assert.equal(eas.build['development-simulator'].extends, 'development');
  assert.equal(eas.build['development-simulator'].ios.simulator, true);
});

test('no live submission: Android submit stays an internal draft, no iOS submit profile', () => {
  assert.deepEqual(eas.submit.production.android, { track: 'internal', releaseStatus: 'draft' });
  assert.equal(eas.submit.production.ios, undefined);
});

test('store metadata draft is Turkish and within App Store limits', () => {
  assert.equal(store.configVersion, 0);
  const info = store.apple.info['tr-TR'];
  assert.ok(info, 'tr-TR listing');
  assert.ok(info.title.length <= 30, 'title ≤ 30');
  assert.ok(info.subtitle.length <= 30, 'subtitle ≤ 30');
  assert.ok(info.description.length <= 4000, 'description ≤ 4000');
  assert.ok(info.keywords.join(',').length <= 100, 'keywords ≤ 100');
  assert.ok(info.releaseNotes.length > 0);
});

test('Play listing drafts fit Google Play limits', () => {
  const play = (f: string) => readFileSync(new URL(`../../../store/play/tr-TR/${f}`, import.meta.url), 'utf8').trim();
  assert.ok(play('title.txt').length <= 30, 'title ≤ 30');
  assert.ok(play('short-description.txt').length <= 80, 'short ≤ 80');
  assert.ok(play('full-description.txt').length <= 4000, 'full ≤ 4000');
});

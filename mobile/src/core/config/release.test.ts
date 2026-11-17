// Sürüm hazırlığı denetimleri (#183): eas.json profilleri, uygulama kimliği,
// sürüm numarası stratejisi ve mağaza meta veri taslakları tutarlı kalmalı.
// Mağaza gönderimi yapılmaz; bu test yalnızca yapılandırmayı denetler.
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { createRequire } from 'node:module';
import { test } from 'node:test';

const read = (p: string) => JSON.parse(readFileSync(new URL(`../../../${p}`, import.meta.url), 'utf8'));

const app = read('app.json').expo;
const pkg = read('package.json');
const eas = read('eas.json');
const require = createRequire(import.meta.url);
const store = require('../../../store/store.config.js');
const appConfig = require('../../../app.config.js');

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

test('submit profiles: Android internal draft, iOS ids are placeholders only (#203)', () => {
  const { android, ios } = eas.submit.production;
  assert.equal(android.track, 'internal');
  assert.equal(android.releaseStatus, 'draft');
  assert.match(android.serviceAccountKeyPath, /^\.\/secrets\//, 'service account key stays out of git');
  // Real App Store Connect ids are an external decision: placeholders until then.
  assert.match(ios.ascAppId, /^TBD_/);
  assert.match(ios.appleTeamId, /^TBD_/);
  assert.equal(ios.metadataPath, './store/store.config.js');
  assert.equal(eas.submit.preview.extends, 'production');
});

test('OTA: one EAS channel per build profile, runtime follows the app version (#203)', () => {
  for (const name of ['development', 'preview', 'production']) assert.equal(eas.build[name].channel, name);
  const off = appConfig({ config: app });
  assert.deepEqual(off.runtimeVersion, { policy: 'appVersion' });
  assert.equal(off.updates.enabled, false, 'no EAS project id: updates off');
  const prev = process.env.EAS_PROJECT_ID;
  process.env.EAS_PROJECT_ID = '00000000-0000-0000-0000-000000000000';
  try {
    const on = appConfig({ config: app });
    assert.equal(on.updates.url, 'https://u.expo.dev/00000000-0000-0000-0000-000000000000');
  } finally {
    if (prev === undefined) delete process.env.EAS_PROJECT_ID;
    else process.env.EAS_PROJECT_ID = prev;
  }
});

test('store placeholders come from the environment and are listed', () => {
  const missing: string[] = store.missingStoreValues();
  assert.ok(missing.includes('STORE_PRIVACY_URL'));
  const readme = readFileSync(new URL('../../../store/README.md', import.meta.url), 'utf8');
  for (const name of Object.values(store.storeEnv as Record<string, string>)) assert.ok(readme.includes(name), `README lists ${name}`);
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

test('Android manifest turns GWP-ASan off (#454: Hermes start-up crash on Android 14)', () => {
  const cfg = appConfig({ config: app });
  assert.equal(typeof cfg.mods?.android?.manifest, 'function');
  const src = readFileSync(new URL('../../../app.config.js', import.meta.url), 'utf8');
  assert.match(src, /'android:gwpAsanMode'\] = 'never'/);
});

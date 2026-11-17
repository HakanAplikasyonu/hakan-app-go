// Sürüm hazırlığı denetimleri (#236, #242; kalıp mobile/src/core/config/release.test.ts):
// uygulama kimliği, eas.json profilleri, OTA, gönderim yer tutucuları ve mağaza
// meta veri taslakları tutarlı kalmalı. Gönderim yapılmaz.
import assert from 'node:assert/strict';
import { createHash } from 'node:crypto';
import { readFileSync } from 'node:fs';
import { createRequire } from 'node:module';
import { test } from 'node:test';

const read = (p: string) => JSON.parse(readFileSync(new URL(`../../../${p}`, import.meta.url), 'utf8'));
const bytes = (p: string) => readFileSync(new URL(`../../../${p}`, import.meta.url));

const app = read('app.json').expo;
const pkg = read('package.json');
const eas = read('eas.json');
const require = createRequire(import.meta.url);
const store = require('../../../store/store.config.js');
const appConfig = require('../../../app.config.js');

test('office app has its own ids and scheme', () => {
  assert.equal(app.ios.bundleIdentifier, 'app.hakan.office');
  assert.equal(app.android.package, 'app.hakan.office');
  assert.equal(app.scheme, 'hakanofis');
  assert.equal(app.slug, 'hakan-ofis');
  assert.notEqual(pkg.name, 'hakan-mobile');
});

test('version is semver and matches package.json; build numbers from EAS', () => {
  assert.match(app.version, /^\d+\.\d+\.\d+$/);
  assert.equal(app.version, pkg.version);
  assert.equal(eas.cli.appVersionSource, 'remote');
  assert.equal(eas.build.production.autoIncrement, true);
  assert.equal(eas.cli.requireCommit, true);
});

test('profiles: development / preview internal, production to the stores, one channel each', () => {
  for (const name of ['development', 'preview', 'production']) {
    assert.equal(eas.build[name].env.EXPO_PUBLIC_APP_ENV, name);
    assert.equal(eas.build[name].channel, name);
  }
  assert.equal(eas.build.development.developmentClient, true);
  assert.equal(eas.build.preview.distribution, 'internal');
  assert.equal(eas.build.production.distribution, 'store');
  assert.equal(eas.build.production.android.buildType, 'app-bundle');
});

test('submit profiles: Android internal draft, iOS ids placeholders, key out of git (#242)', () => {
  const { android, ios } = eas.submit.production;
  assert.equal(android.track, 'internal');
  assert.equal(android.releaseStatus, 'draft');
  assert.match(android.serviceAccountKeyPath, /^\.\/secrets\//);
  assert.match(readFileSync(new URL('../../../.gitignore', import.meta.url), 'utf8'), /^secrets\/$/m);
  assert.match(ios.ascAppId, /^TBD_/);
  assert.match(ios.appleTeamId, /^TBD_/);
  assert.equal(ios.metadataPath, './store/store.config.js');
  assert.equal(eas.submit.preview.extends, 'production');
});

test('OTA follows the app version; off without an EAS project; app links for /t/ only with a domain', () => {
  const off = appConfig({ config: app });
  assert.deepEqual(off.runtimeVersion, { policy: 'appVersion' });
  assert.equal(off.updates.enabled, false);
  assert.equal(off.ios.associatedDomains, undefined);
  const saved = { id: process.env.EAS_OFFICE_PROJECT_ID, domain: process.env.EXPO_PUBLIC_APP_LINK_DOMAIN };
  process.env.EAS_OFFICE_PROJECT_ID = '00000000-0000-0000-0000-000000000000';
  process.env.EXPO_PUBLIC_APP_LINK_DOMAIN = 'https://app.example.com/';
  try {
    const on = appConfig({ config: app });
    assert.equal(on.updates.url, 'https://u.expo.dev/00000000-0000-0000-0000-000000000000');
    assert.deepEqual(on.ios.associatedDomains, ['applinks:app.example.com']);
    assert.equal(on.android.intentFilters[0].data[0].pathPrefix, '/t/');
    assert.equal(on.ios.bundleIdentifier, 'app.hakan.office');
  } finally {
    for (const [k, v] of [['EAS_OFFICE_PROJECT_ID', saved.id], ['EXPO_PUBLIC_APP_LINK_DOMAIN', saved.domain]] as const) {
      if (v === undefined) delete process.env[k];
      else process.env[k] = v;
    }
  }
});

test('icon and splash differ from the mükellef app (#242)', () => {
  for (const f of ['icon.png', 'adaptive-icon.png', 'splash-icon.png', 'favicon.png']) {
    const office = createHash('sha256').update(bytes(`assets/${f}`)).digest('hex');
    const mukellef = createHash('sha256').update(bytes(`../mobile/assets/${f}`)).digest('hex');
    assert.notEqual(office, mukellef, `${f} is the mükellef app's`);
  }
  assert.equal(app.android.adaptiveIcon.backgroundColor, '#2448FF');
});

test('store placeholders come from the environment and are all listed', () => {
  const missing: string[] = store.missingStoreValues();
  assert.ok(missing.includes('STORE_OFFICE_PRIVACY_URL'));
  const readme = readFileSync(new URL('../../../store/README.md', import.meta.url), 'utf8');
  for (const name of Object.values(store.storeEnv as Record<string, string>)) assert.ok(readme.includes(name), `README lists ${name}`);
});

test('store metadata draft is Turkish and within limits', () => {
  const info = store.apple.info['tr-TR'];
  assert.ok(info.title.length <= 30 && info.subtitle.length <= 30);
  assert.ok(info.description.length <= 4000);
  assert.ok(info.keywords.join(',').length <= 100);
  const play = (f: string) => readFileSync(new URL(`../../../store/play/tr-TR/${f}`, import.meta.url), 'utf8').trim();
  assert.ok(play('title.txt').length <= 30, 'Play title ≤ 30');
  assert.ok(play('short-description.txt').length <= 80, 'Play short ≤ 80');
  assert.ok(play('full-description.txt').length <= 4000, 'Play full ≤ 4000');
  assert.ok(play('release-notes.txt').length <= 500, 'Play release notes ≤ 500');
});

test('Android manifest turns GWP-ASan off (#454: Hermes start-up crash on Android 14)', () => {
  const cfg = appConfig({ config: app });
  assert.equal(typeof cfg.mods?.android?.manifest, 'function');
  const src = readFileSync(new URL('../../../app.config.js', import.meta.url), 'utf8');
  assert.match(src, /'android:gwpAsanMode'\] = 'never'/);
});

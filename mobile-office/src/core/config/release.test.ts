// Uygulama kimliği ve eas.json profilleri (#236): mükellef uygulamasıyla
// (mobile/, app.hakan.mukellef) karışmamalı.
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { test } from 'node:test';

const read = (p: string) => JSON.parse(readFileSync(new URL(`../../../${p}`, import.meta.url), 'utf8'));

const app = read('app.json').expo;
const pkg = read('package.json');
const eas = read('eas.json');

test('office app has its own ids and scheme', () => {
  assert.equal(app.ios.bundleIdentifier, 'app.hakan.office');
  assert.equal(app.android.package, 'app.hakan.office');
  assert.equal(app.scheme, 'hakanofis');
  assert.equal(app.slug, 'hakan-ofis');
  assert.notEqual(pkg.name, 'hakan-mobile');
});

test('version is semver and matches package.json', () => {
  assert.match(app.version, /^\d+\.\d+\.\d+$/);
  assert.equal(app.version, pkg.version);
});

test('eas profiles exist and build numbers come from EAS', () => {
  for (const p of ['development', 'preview', 'production']) {
    assert.ok(eas.build[p], `missing build profile ${p}`);
    assert.equal(eas.build[p].env.EXPO_PUBLIC_APP_ENV, p);
  }
  assert.equal(eas.cli.appVersionSource, 'remote');
  assert.equal(eas.build.production.autoIncrement, true);
});

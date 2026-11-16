import assert from 'node:assert/strict';
import { test } from 'node:test';

import {
  defaultThemePreference,
  guvenKademesi,
  parseThemePreference,
  resolveThemeName,
  serializeThemePreference,
} from './theme-pref.ts';
import { colors } from './tokens.ts';

test('varsayılan tercih M3 + sistemi izle', () => {
  assert.deepEqual(defaultThemePreference, { palette: 'm3', mode: 'system' });
  assert.equal(resolveThemeName(defaultThemePreference, 'dark'), 'm3-dark');
  assert.equal(resolveThemeName(defaultThemePreference, 'light'), 'm3-light');
  assert.equal(resolveThemeName(defaultThemePreference, null), 'm3-light');
});

test('açık seçilen kip sistemi ezer', () => {
  assert.equal(resolveThemeName({ palette: 'n3', mode: 'dark' }, 'light'), 'n3-dark');
  assert.equal(resolveThemeName({ palette: 'n3', mode: 'light' }, 'dark'), 'n3-light');
});

test('bozuk ya da eksik kayıt varsayılana döner', () => {
  assert.deepEqual(parseThemePreference(null), defaultThemePreference);
  assert.deepEqual(parseThemePreference('{'), defaultThemePreference);
  assert.deepEqual(parseThemePreference('{"palette":"x","mode":"y"}'), defaultThemePreference);
  const p = { palette: 'n3', mode: 'dark' } as const;
  assert.deepEqual(parseThemePreference(serializeThemePreference(p)), p);
});

test('dört temanın hepsi tanımlı; N3 yeşili yalnız dolgu, üstünde siyah', () => {
  for (const name of ['m3-light', 'm3-dark', 'n3-light', 'n3-dark'] as const) assert.ok(colors[name].brand);
  assert.equal(colors['n3-light'].onBrand, '#0A0A0A');
  assert.equal(colors['n3-light'].brandText, '#067A12');
});

test('güven kademeleri: .6 / .9 eşikleri, kural her zaman yüksek', () => {
  assert.equal(guvenKademesi(0.59), 'dusuk');
  assert.equal(guvenKademesi(0.6), 'orta');
  assert.equal(guvenKademesi(0.89), 'orta');
  assert.equal(guvenKademesi(0.9), 'yuksek');
  assert.equal(guvenKademesi(0.1, 'kural'), 'yuksek');
});

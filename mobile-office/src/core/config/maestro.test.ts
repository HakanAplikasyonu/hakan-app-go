// Maestro duman akışlarının sözdizimi denetimi (#242; kalıp
// mobile/src/core/config/maestro.test.ts, #203). Emülatörde çalıştırma CI'da
// yok (gerekçe: README.md, "Duman testleri"); bu test akışların bozulmadan
// kalmasını sağlar: YAML geçerli, appId doğru, yalnızca bilinen komutlar,
// config sırası var olan akışları gösterir, env değişkenleri belgeli.
import assert from 'node:assert/strict';
import { readdirSync, readFileSync } from 'node:fs';
import { test } from 'node:test';

import { parse, parseAllDocuments } from 'yaml';

const dir = new URL('../../../.maestro/', import.meta.url);
const flowsDir = new URL('flows/', dir);
const app = JSON.parse(readFileSync(new URL('../../../app.json', import.meta.url), 'utf8')).expo;

const COMMANDS = new Set([
  'launchApp', 'stopApp', 'clearState', 'tapOn', 'longPressOn', 'doubleTapOn', 'inputText', 'eraseText',
  'hideKeyboard', 'pressKey', 'back', 'scroll', 'scrollUntilVisible', 'swipe', 'assertVisible',
  'assertNotVisible', 'assertTrue', 'extendedWaitUntil', 'waitForAnimationToEnd', 'runFlow', 'openLink',
  'takeScreenshot', 'copyTextFrom', 'pasteText', 'evalScript', 'repeat',
]);
const ALLOWED_ENV = new Set(['MAESTRO_EMAIL', 'MAESTRO_PASSWORD']);
const ORDER = ['login', 'select-office', 'dashboard', 'review-approve', 'task-create', 'signout'];

function flows() {
  return readdirSync(flowsDir)
    .filter((f) => f.endsWith('.yaml'))
    .map((f) => {
      const text = readFileSync(new URL(f, flowsDir), 'utf8');
      const docs = parseAllDocuments(text);
      for (const d of docs) assert.equal(d.errors.length, 0, `${f}: ${d.errors.map((e) => e.message).join('; ')}`);
      return { file: f, text, header: docs[0]?.toJS(), steps: docs[1]?.toJS() };
    });
}

function checkSteps(file: string, steps: unknown) {
  assert.ok(Array.isArray(steps) && steps.length > 0, `${file}: no commands`);
  for (const step of steps as unknown[]) {
    const name = typeof step === 'string' ? step : Object.keys(step as object)[0];
    assert.ok(COMMANDS.has(name), `${file}: unknown command ${name}`);
    if (name === 'runFlow' && typeof step === 'object') {
      const body = (step as Record<string, { commands?: unknown }>).runFlow;
      if (body && typeof body === 'object' && 'commands' in body) checkSteps(file, body.commands);
    }
  }
}

test('every flow is valid Maestro YAML for the office app', () => {
  const all = flows();
  assert.equal(all.length, ORDER.length);
  for (const f of all) {
    assert.equal(f.header?.appId, app.ios.bundleIdentifier, `${f.file}: appId`);
    assert.equal(f.header?.appId, app.android.package, `${f.file}: appId`);
    assert.ok(f.header?.name, `${f.file}: name`);
    checkSteps(f.file, f.steps);
    for (const m of f.text.matchAll(/\$\{([A-Z_]+)\}/g)) assert.ok(ALLOWED_ENV.has(m[1]), `${f.file}: undocumented env ${m[1]}`);
    for (const m of f.text.matchAll(/openLink: "([^"]+)"/g)) assert.ok(m[1].startsWith(`${app.scheme}://`), `${f.file}: link ${m[1]}`);
  }
});

test('config runs login, dashboard, review approval and task creation in order', () => {
  const cfg = parse(readFileSync(new URL('config.yaml', dir), 'utf8'));
  const names = new Set(flows().map((f) => f.header.name));
  assert.deepEqual(cfg.executionOrder.flowsOrder, ORDER);
  for (const n of ORDER) assert.ok(names.has(n), `missing flow ${n}`);
});

test('the ids the flows tap exist in the screens', () => {
  const src = (p: string) => readFileSync(new URL(`../../app/${p}`, import.meta.url), 'utf8');
  const screens = [
    src('login.tsx'),
    src('(app)/(tabs)/_layout.tsx'),
    src('(app)/(tabs)/index.tsx'),
    src('(app)/(tabs)/clients/index.tsx'),
    src('(app)/(tabs)/review/index.tsx'),
    src('(app)/(tabs)/notifications.tsx'),
    src('(app)/tasks/new.tsx'),
  ].join('\n');
  // Sabit testID ("x" ya da 'x') ya da önekle kurulan testID (`x-${...}`, testIDOnek="x").
  const has = (id: string) => {
    if (screens.includes(`"${id}"`) || screens.includes(`'${id}'`)) return true;
    const prefix = id.slice(0, id.lastIndexOf('-'));
    return !!prefix && (screens.includes(`\`${prefix}-\${`) || screens.includes(`testIDOnek="${prefix}"`));
  };
  for (const f of flows()) {
    for (const m of f.text.matchAll(/id: "([^"]+)"/g)) assert.ok(has(m[1]), `${f.file}: no testID ${m[1]}`);
  }
});

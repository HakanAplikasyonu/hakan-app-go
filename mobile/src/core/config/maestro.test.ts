// Maestro duman akışlarının sözdizimi denetimi (#203). Emülatörde çalıştırma
// CI'da yok (gerekçe: mobile/README.md, "Duman testleri"); bu test akışların
// bozulmadan kalmasını sağlar: YAML geçerli, appId doğru, yalnızca bilinen
// komutlar, config sırası var olan akışları gösterir, env değişkenleri belgeli.
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
  'takeScreenshot', 'copyTextFrom', 'pasteText', 'evalScript', 'repeat', 'setAirplaneMode',
]);
const ALLOWED_ENV = new Set(['MAESTRO_EMAIL', 'MAESTRO_PASSWORD']);

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

test('every flow is valid Maestro YAML for this app', () => {
  const all = flows();
  assert.ok(all.length >= 6, 'login, select-taxpayer, capture, tasks, payroll, signout');
  for (const f of all) {
    assert.equal(f.header?.appId, app.ios.bundleIdentifier, `${f.file}: appId`);
    assert.equal(f.header?.appId, app.android.package, `${f.file}: appId`);
    assert.ok(f.header?.name, `${f.file}: name`);
    checkSteps(f.file, f.steps);
    for (const m of f.text.matchAll(/\$\{([A-Z_]+)\}/g)) assert.ok(ALLOWED_ENV.has(m[1]), `${f.file}: undocumented env ${m[1]}`);
  }
});

test('config runs the smoke flows in order and they all exist', () => {
  const cfg = parse(readFileSync(new URL('config.yaml', dir), 'utf8'));
  const names = new Set(flows().map((f) => f.header.name));
  assert.deepEqual(cfg.executionOrder.flowsOrder, ['login', 'select-taxpayer', 'capture', 'tasks', 'payroll', 'signout']);
  for (const n of cfg.executionOrder.flowsOrder) assert.ok(names.has(n), `missing flow ${n}`);
});

test('the ids the flows tap exist in the app (#328)', () => {
  const appDir = new URL('../../app/', import.meta.url);
  const read = (dir: URL): string =>
    readdirSync(dir, { withFileTypes: true })
      .map((e) => (e.isDirectory() ? read(new URL(`${e.name}/`, dir)) : e.name.endsWith('.tsx') ? readFileSync(new URL(e.name, dir), 'utf8') : ''))
      .join('\n');
  // Ekranlar app/ altında; bazı parçalar features/ altında (ör. ilk açılış, #332).
  const src = read(appDir) + read(new URL('../../features/', import.meta.url)) + readFileSync(new URL('../../../../packages/mobile-core/src/ui/sekme-cubugu.tsx', import.meta.url), 'utf8');
  // Sabit testID ("x" ya da 'x') ya da önekle kurulan testID (`x-${...}`, testIDOnek="x").
  const has = (id: string) => {
    if (src.includes(`"${id}"`) || src.includes(`'${id}'`)) return true;
    const prefix = id.slice(0, id.lastIndexOf('-'));
    return !!prefix && (src.includes(`\`${prefix}-\${`) || src.includes(`testIDOnek="${prefix}"`));
  };
  for (const f of flows()) {
    for (const m of f.text.matchAll(/id: "([^"]+)"/g)) assert.ok(has(m[1]), `${f.file}: no testID ${m[1]}`);
  }
});

// M15 ekranlarında ham alan adı ve sabit renk kalmasın (#454, #367 kalıbı).
// 1) Ekranda gösterilen her enum değerinin Türkçe etiketi var ve etiket
//    kümesi API sözleşmesindeki (backend/docs/openapi.yaml) enum ile aynı:
//    sunucu yeni bir değer eklerse ekranda ham anahtar görünmeden test düşer.
// 2) M15 ekranlarında renk sabiti yok; renkler tema token'larından.
import assert from 'node:assert/strict';
import { readdirSync, readFileSync } from 'node:fs';
import { test } from 'node:test';

import { parse } from 'yaml';

import { categoryLabels, isgKindLabels } from '../../features/archive/format.ts';
import { actionLabels, statusLabels, stepLabels } from '../../features/copilot/format.ts';
import { kindLabels, paymentLabels } from '../../features/payroll/format.ts';

const spec = parse(readFileSync(new URL('../../../../backend/docs/openapi.yaml', import.meta.url), 'utf8'));
const schemas = spec.components.schemas as Record<string, { properties?: Record<string, { enum?: unknown[]; oneOf?: unknown[] }>; enum?: unknown[] }>;

function enumOf(schema: string, prop?: string): string[] {
  const s = schemas[schema];
  assert.ok(s, `schema ${schema}`);
  const e = prop ? s.properties?.[prop]?.enum : s.enum;
  assert.ok(Array.isArray(e), `${schema}.${prop ?? ''} enum`);
  return (e as unknown[]).filter((v): v is string => typeof v === 'string').sort();
}

const keys = (o: object) => Object.keys(o).sort();

test('labels cover exactly the API enums (no raw keys on screen)', () => {
  assert.deepEqual(keys(categoryLabels), enumOf('ArchiveDocument', 'category'));
  assert.deepEqual(keys(isgKindLabels), enumOf('ArchiveDocument', 'isg_kind'));
  assert.deepEqual(keys(kindLabels), enumOf('PayrollDocument', 'kind'));
  assert.deepEqual(keys(paymentLabels), enumOf('PayrollRun', 'payment_status'));
  assert.deepEqual(keys(stepLabels), enumOf('CopilotStep', 'key'));
  assert.deepEqual(keys(statusLabels), enumOf('CopilotStep', 'status'));
  assert.deepEqual(keys(actionLabels), enumOf('CopilotAction'));
});

test('labels are Turkish sentences, not keys', () => {
  for (const map of [categoryLabels, isgKindLabels, kindLabels, stepLabels, actionLabels]) {
    for (const [k, v] of Object.entries(map)) {
      assert.ok(typeof v === 'string' && v.length > 0, k);
      assert.doesNotMatch(v, /_/, `${k}: "${v}" looks like a raw key`);
      assert.notEqual(v, k);
    }
  }
});

// Kapsam: M15'te gelen ofis mobil ekranları (#393, #392).
const M15 = ['features/payroll', 'features/mizan', 'features/archive', 'features/files', 'features/documents', 'features/copilot', 'app/(app)/belgeler', 'app/(app)/siradaki-is.tsx', 'components/load-error.tsx'];

function sources(rel: string): { file: string; text: string }[] {
  const url = new URL(`../../${rel}`, import.meta.url);
  if (rel.endsWith('.tsx') || rel.endsWith('.ts')) return [{ file: rel, text: readFileSync(url, 'utf8') }];
  return readdirSync(url)
    .filter((f) => /\.tsx?$/.test(f) && !f.endsWith('.test.ts'))
    .map((f) => ({ file: `${rel}/${f}`, text: readFileSync(new URL(`${rel}/${f}`, new URL('../../', import.meta.url)), 'utf8') }));
}

test('M15 screens take colours from theme tokens, no literals', () => {
  const files = M15.flatMap(sources);
  assert.ok(files.length >= 15, `scanned ${files.length} files`);
  for (const { file, text } of files) {
    const code = text.replace(/\/\*[\s\S]*?\*\//g, '').replace(/\/\/.*$/gm, '');
    assert.doesNotMatch(code, /['"`]#[0-9a-fA-F]{3,8}['"`]/, `${file}: hex colour`);
    assert.doesNotMatch(code, /\brgba?\(/, `${file}: rgb() colour`);
    assert.doesNotMatch(code, /(color|Color|renk)\s*[:=]\s*['"`][a-z]+['"`]/, `${file}: named colour`);
  }
});

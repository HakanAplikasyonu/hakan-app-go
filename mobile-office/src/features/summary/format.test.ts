import assert from 'node:assert/strict';
import { test } from 'node:test';

import {
  cents,
  dayRanges,
  directionTotals,
  gapLines,
  integrationIssues,
  istanbulToday,
  missingPeriod,
  tl,
  vatBreakdown,
  vatResult,
  zGapDays,
} from './format.ts';

const rate = (r: string, ob: string, ov: string, ib: string, iv: string, ow = '0.00') => ({
  tax_rate: r,
  output_base: ob,
  output_vat: ov,
  output_withholding: ow,
  input_base: ib,
  input_vat: iv,
  input_withholding: '0.00',
});

test('cents and tl work in kuruş without float drift', () => {
  assert.equal(cents('0.10') + cents('0.20'), 30);
  assert.equal(cents('1234.56'), 123456);
  assert.equal(cents(''), 0);
  assert.equal(cents(null), 0);
  assert.equal(cents('abc'), 0);
  assert.equal(tl(123450), '1.234,50 ₺');
  assert.equal(tl(-5000), '-50,00 ₺');
});

test('vatBreakdown sorts by rate, drops empty rates and nets each rate', () => {
  const rows = vatBreakdown([rate('1.00', '0', '0', '0', '0'), rate('10.00', '1000.00', '100.00', '500.00', '50.00'), rate('20.00', '2000.00', '400.00', '3000.00', '600.00')]);
  assert.deepEqual(
    rows.map((r) => [r.rate, r.label, r.net]),
    [
      ['20.00', 'KDV %20', -20000],
      ['10.00', 'KDV %10', 5000],
    ],
  );
  assert.deepEqual(vatBreakdown(null), []);
});

test('vatResult says payable, carried forward or nothing', () => {
  const base = { output_vat: '0', input_vat: '0', carried_in: '0' };
  assert.deepEqual(vatResult({ ...base, payable: '1500.00', carry_out: '0.00' }), { kind: 'payable', amount: 150000, title: 'Ödenecek KDV', text: '1.500,00 ₺ ödenecek' });
  assert.equal(vatResult({ ...base, payable: '0.00', carry_out: '320.40' })?.kind, 'carry');
  assert.equal(vatResult({ ...base, payable: '0.00', carry_out: '320.40' })?.text, '320,40 ₺ sonraki aya devreder');
  assert.equal(vatResult({ ...base, payable: '0.00', carry_out: '0.00' })?.kind, 'zero');
  assert.equal(vatResult(undefined), null);
});

test('directionTotals adds the withheld sales VAT back to the outgoing invoice total', () => {
  const t = directionTotals([rate('20.00', '1000.00', '160.00', '200.00', '40.00', '40.00'), rate('10.00', '100.00', '10.00', '0', '0')]);
  assert.deepEqual(t.outgoing, { base: 110000, vat: 21000, total: 131000 });
  assert.deepEqual(t.incoming, { base: 20000, vat: 4000, total: 24000 });
});

test('zGapDays lists the days without a Z report up to yesterday', () => {
  assert.deepEqual(zGapDays('2026-11', ['2026-11-01', '2026-11-02', '2026-11-04'], '2026-11-06'), [3, 5]);
  // Geçmiş ay: ay sonuna kadar.
  assert.equal(zGapDays('2026-02', [], '2026-11-06').length, 28);
  assert.equal(zGapDays('2024-02', [], '2026-11-06').length, 29);
  // Başka ayın tarihi sayılmaz; gelecek ay boş; ayın ilk günü boş.
  assert.deepEqual(zGapDays('2026-10', ['2026-11-01'], '2026-10-03'), [1, 2]);
  assert.deepEqual(zGapDays('2026-12', [], '2026-11-06'), []);
  assert.deepEqual(zGapDays('2026-11', [], '2026-11-01'), []);
  assert.deepEqual(zGapDays('bozuk', [], '2026-11-06'), []);
});

test('dayRanges collapses consecutive days', () => {
  assert.equal(dayRanges([9, 1, 2, 3, 5, 8]), '1–3, 5, 8–9');
  assert.equal(dayRanges([4]), '4');
  assert.equal(dayRanges([]), '');
});

test('istanbulToday uses Turkish time', () => {
  assert.equal(istanbulToday(new Date('2026-11-30T22:30:00Z')), '2026-12-01');
});

test('integrationIssues reports failing connections in Turkish and skips disabled ones', () => {
  const out = integrationIssues({
    einvoice: [
      { integrator_name: 'Örnek Entegratör', state: 'auth_failed' },
      { integrator_name: 'Sağlam Entegratör', state: 'active' },
    ],
    bank: [{ bank_name: 'Örnek Bankası', status: 'active', error: { message: 'timeout' } }],
    easytrade: [{ status: 'disabled', error: 'x' }],
    kolaybi: [{ status: 'rate_limited', company_name: 'Deneme Ltd.' }],
    parasut: [{ status: 'active' }],
  });
  assert.deepEqual(out, [
    { name: 'e-Fatura · Örnek Entegratör', problem: 'Giriş bilgileri reddedildi' },
    { name: 'Banka · Örnek Bankası', problem: 'Son eşitleme hata verdi' },
    { name: 'KolayBi · Deneme Ltd.', problem: 'İstek sınırına takıldı' },
  ]);
  assert.deepEqual(integrationIssues({}), []);
});

test('gapLines puts missing bank statements first, then Z gaps and sync errors', () => {
  const lines = gapLines({
    missing: {
      period: '2026-10',
      items: [
        { kind: 'z_reports', name: '', requested: false },
        { kind: 'bank_statement', name: 'Örnek Bankası TL', requested: true },
        { kind: 'unknown', name: '', requested: false },
      ],
    },
    zGaps: { period: '2026-11', days: [3, 4, 5, 9] },
    integrations: [{ name: 'Banka · Örnek Bankası', problem: 'Son eşitleme hata verdi' }],
  });
  assert.deepEqual(
    lines.map((l) => [l.title, l.detail, l.tone]),
    [
      ['Banka ekstresi eksik', 'Örnek Bankası TL · Ekim 2026 · mükelleften istendi', 'danger'],
      ['Z raporu gelmedi', 'Ekim 2026 · henüz istenmedi', 'warning'],
      ['Z raporu olmayan 4 gün', 'Kasım 2026: 3–5, 9', 'warning'],
      ['Eşitleme hatası', 'Banka · Örnek Bankası: Son eşitleme hata verdi', 'danger'],
    ],
  );
  assert.deepEqual(gapLines({}), []);
  assert.deepEqual(gapLines({ zGaps: { period: '2026-11', days: [] } }), []);
  assert.equal(new Set(lines.map((l) => l.key)).size, lines.length);
});

test('missingPeriod looks at last month while the current month is open', () => {
  assert.equal(missingPeriod('2026-11', '2026-11'), '2026-10');
  assert.equal(missingPeriod('2026-01', '2026-01'), '2025-12');
  assert.equal(missingPeriod('2026-09', '2026-11'), '2026-09');
});

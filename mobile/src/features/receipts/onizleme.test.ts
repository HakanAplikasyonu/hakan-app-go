import assert from 'node:assert/strict';
import { test } from 'node:test';

import {
  addableRates,
  addRow,
  canSend,
  checkPreview,
  editRow,
  fillTax,
  formatAmount,
  isoToTr,
  parseAmount,
  previewCorrections,
  previewFrom,
  removeRow,
  rowsSum,
  trToIso,
} from './onizleme.ts';

const f = (value: string, raw = value, valid = true, confidence = 0.95) => ({ value, raw, valid, confidence });

// A market receipt: %1 food and %20 cleaning products (KDV dahil 101 + 120).
const fields = {
  vendor_name: f('ÖRNEK MARKET A.Ş.'),
  date: f('2026-10-14', '14.10.2026'),
  total_amount: f('221.00', '221,00'),
  tax_amount: f('21.00', '21,00'),
  vat_1_total: f('101.00', '101,00'),
  vat_1_tax: f('1.00', '1,00'),
  vat_20_total: f('120.00', '120,00'),
  vat_20_tax: f('20.00', '20,00'),
};

test('amounts and dates in Turkish format', () => {
  assert.equal(parseAmount('1.234,56'), 1234.56);
  assert.equal(parseAmount('1234.56'), 1234.56);
  assert.equal(parseAmount('1,234.56'), 1234.56);
  assert.equal(parseAmount('*₺12,00'), 12);
  assert.equal(parseAmount('1.234.567'), 1234567);
  assert.equal(parseAmount(''), null);
  assert.equal(parseAmount('on iki'), null);
  assert.equal(formatAmount(1234.5), '1.234,50');
  assert.equal(formatAmount(0.004), '0,00');
  assert.equal(isoToTr('2026-10-14'), '14.10.2026');
  assert.equal(trToIso('14.10.2026'), '2026-10-14');
  assert.equal(trToIso('1/2/2026'), '2026-02-01');
  assert.equal(trToIso('31.02.2026'), null);
  assert.equal(trToIso('2026'), null);
});

test('VAT rate rows are read separately with matrah and VAT', () => {
  const p = previewFrom(fields);
  assert.equal(p.vendor, 'ÖRNEK MARKET A.Ş.');
  assert.equal(p.date, '14.10.2026');
  assert.equal(p.total, '221,00');
  assert.deepEqual(p.rows, [
    { rate: '1', base: '100,00', tax: '1,00' },
    { rate: '20', base: '100,00', tax: '20,00' },
  ]);
  assert.deepEqual(rowsSum(p.rows), { base: 200, tax: 21, total: 221 });
  assert.deepEqual(checkPreview(p).errors, {});
  assert.equal(canSend(p), true);
});

test('a rate whose VAT was not read is computed from its total and marked', () => {
  const p = previewFrom({ ...fields, vat_20_tax: undefined });
  assert.deepEqual(p.rows[1], { rate: '20', base: '100,00', tax: '20,00', estimated: true });
});

test('one VAT read without rows becomes one row', () => {
  const p = previewFrom({ total_amount: f('120.00'), tax_amount: f('20.00'), tax_rate: f('20') });
  assert.deepEqual(p.rows, [{ rate: '20', base: '100,00', tax: '20,00' }]);
  assert.deepEqual(previewFrom({ total_amount: f('120.00') }).rows, []);
});

test('rows are edited, added and removed', () => {
  let rows = previewFrom(fields).rows;
  assert.deepEqual(addableRates(rows), ['10']);
  rows = addRow(rows, '10');
  assert.deepEqual(rows.map((r) => r.rate), ['1', '10', '20']);
  assert.equal(addRow(rows, '10').length, 3);
  rows = editRow(rows, '10', { base: '50,00' });
  rows = fillTax(rows, '10');
  assert.deepEqual(rows[1], { rate: '10', base: '50,00', tax: '5,00' });
  assert.deepEqual(removeRow(rows, '10').map((r) => r.rate), ['1', '20']);
});

test('the rows must add up to the total', () => {
  const p = previewFrom(fields);
  const off = { ...p, rows: editRow(p.rows, '20', { base: '90,00', tax: '18,00' }) };
  const check = checkPreview(off);
  assert.match(check.errors.sum, /209,00 TL.*221,00 TL/);
  assert.equal(canSend(off), false);
  // A kuruş of rounding is fine.
  assert.equal(canSend({ ...p, total: '221,03' }), true);
  // An empty row or field stops sending with a Turkish message.
  const empty = { ...p, vendor: ' ', date: '14/13/2026', rows: addRow(p.rows, '10') };
  const e = checkPreview(empty).errors;
  assert.ok(e.vendor && e.date && e.row_10);
  // A VAT that does not match its rate is a note, not an error.
  const odd = { ...p, total: '222,00', rows: editRow(p.rows, '20', { tax: '21,00' }) };
  assert.deepEqual(checkPreview(odd).errors, {});
  assert.match(checkPreview(odd).notes['20'], /%20/);
});

test('only changed fields are sent; rows as total incl. VAT and VAT', () => {
  const original = previewFrom(fields);
  assert.deepEqual(previewCorrections(fields, original, original), []);

  const edited = {
    vendor: 'Örnek Market',
    date: '15.10.2026',
    total: '231,00',
    rows: addRow(editRow(original.rows, '20', { base: '100,00', tax: '20,00' }), '10').map((r) =>
      r.rate === '10' ? { rate: '10', base: '10,00', tax: '1,00' } : r,
    ),
  };
  assert.deepEqual(previewCorrections(fields, original, edited), [
    { field: 'vendor_name', value: 'Örnek Market' },
    { field: 'date', value: '2026-10-15' },
    { field: 'total_amount', value: '231.00' },
    { field: 'vat_10_total', value: '11.00' },
    { field: 'vat_10_tax', value: '1.00' },
    { field: 'tax_amount', value: '22.00' },
  ]);

  // A removed rate goes as total 0; the VAT follows the remaining rows.
  const removed = { ...original, total: '120,00', rows: removeRow(original.rows, '1') };
  assert.deepEqual(previewCorrections(fields, original, removed), [
    { field: 'total_amount', value: '120.00' },
    { field: 'vat_1_total', value: '0' },
    { field: 'tax_amount', value: '20.00' },
  ]);

  // A computed row is confirmed by sending it.
  const est = { ...fields, vat_20_tax: undefined };
  const p = previewFrom(est);
  assert.deepEqual(previewCorrections(est, p, p), [
    { field: 'vat_20_total', value: '120.00' },
    { field: 'vat_20_tax', value: '20.00' },
  ]);
});

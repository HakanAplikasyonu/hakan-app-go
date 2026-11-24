import assert from 'node:assert/strict';
import { test } from 'node:test';

import {
  accountQuery,
  accountSourceLabel,
  amountLabel,
  bankMatchLabel,
  bankStatus,
  dateLabel,
  invoiceStatus,
  isDirection,
  isMonthKind,
  isPeriod,
  listTitle,
  pastPeriods,
  RECORD_TABS,
  paymentMethodLabel,
  rateLabel,
  receiptReadChip,
  receiptStatus,
  vatRatesLabel,
  reviewFieldLabel,
  reviewSummary,
  shortPeriodLabel,
  visibleRecordTabs,
} from './format.ts';

test('pastPeriods counts back from the current Istanbul month', () => {
  // 2026-01-31 22:30 UTC is already February in Istanbul (UTC+3).
  assert.deepEqual(pastPeriods(3, new Date('2026-01-31T22:30:00Z')), ['2026-02', '2026-01', '2025-12']);
  assert.deepEqual(pastPeriods(2, new Date('2026-01-15T10:00:00Z')), ['2026-01', '2025-12']);
  assert.equal(pastPeriods(12, new Date('2026-11-23T10:00:00Z')).length, 12);
  assert.equal(pastPeriods(0, new Date('2026-11-23T10:00:00Z')).length, 1);
});

test('isPeriod and isMonthKind', () => {
  assert.ok(isPeriod('2026-11'));
  assert.ok(!isPeriod('2026-13'));
  assert.ok(!isPeriod('2026-1'));
  assert.ok(!isPeriod(undefined));
  assert.ok(isMonthKind('zreports'));
  assert.ok(!isMonthKind('payroll'));
});

test('shortPeriodLabel', () => {
  assert.equal(shortPeriodLabel('2026-11'), 'Kas 2026');
  assert.equal(shortPeriodLabel('2026-01'), 'Oca 2026');
  assert.equal(shortPeriodLabel('x'), 'x');
});

test('card status lines', () => {
  assert.equal(invoiceStatus(undefined, 1), 'Yükleniyor…');
  assert.equal(invoiceStatus(0, 0), 'Bu ay kayıt yok');
  assert.equal(invoiceStatus(8, 3), 'Gelen 3 · Giden 5');
  assert.equal(receiptStatus(undefined), 'Yükleniyor…');
  assert.equal(receiptStatus({ total: 0, pending: 0 }), 'Bu ay kayıt yok');
  assert.equal(receiptStatus({ total: 5, pending: 2 }), '2 onay bekliyor · 3 işlendi');
  assert.equal(receiptStatus({ total: 5, pending: 0 }), 'Hepsi işlendi');
  assert.equal(bankStatus({ total: 10, matched: 6, unmatched: 3, suggested: 1 }), '4 hareket bekliyor · 6 eşleşti');
  assert.equal(bankStatus({ total: 4, matched: 4, unmatched: 0, suggested: 0 }), 'Hepsi işlendi');
  assert.equal(bankStatus({ total: 0, matched: 0, unmatched: 0, suggested: 0 }), 'Bu ay kayıt yok');
});

test('review fields are named in Turkish', () => {
  assert.equal(reviewFieldLabel('tax_amount'), 'KDV tutarı');
  assert.equal(reviewFieldLabel('vat_20_tax'), '%20 KDV');
  assert.equal(reviewFieldLabel('vat_10_total'), '%10 toplam');
  assert.equal(reviewFieldLabel('something_new'), 'Diğer alan');
  assert.equal(reviewSummary([]), null);
  assert.equal(reviewSummary(undefined), null);
  assert.equal(reviewSummary(['date', 'total_amount', 'date']), 'Kontrol et · Fiş tarihi, Fiş toplamı');
});

test('labels', () => {
  assert.equal(accountSourceLabel('vendor_memory'), 'Bu satıcının son onaylı masrafındaki hesap');
  assert.equal(accountSourceLabel(null), null);
  assert.equal(paymentMethodLabel('card'), 'Kart');
  assert.equal(paymentMethodLabel('barter'), 'Diğer');
  assert.equal(paymentMethodLabel(null), null);
  assert.equal(bankMatchLabel({ match_status: 'unmatched', booked_account_code: '770' }), 'Hesaba işlendi');
  assert.equal(bankMatchLabel({ match_status: 'suggested' }), 'Öneri var');
});

test('values', () => {
  assert.equal(dateLabel('2026-11-20'), '20.11.2026');
  assert.equal(dateLabel('2026-11-20T10:00:00Z'), '20.11.2026');
  assert.equal(dateLabel(null), '—');
  assert.equal(amountLabel('1234.5'), '1.234,50 ₺');
  assert.equal(amountLabel('10', 'EUR'), '10,00 EUR');
  assert.equal(amountLabel(null), '—');
  assert.equal(rateLabel('20.00'), '%20');
  assert.equal(rateLabel('1.5'), '%1,5');
  assert.equal(rateLabel(null), '—');
  assert.equal(accountQuery(' 7 '), undefined);
  assert.equal(accountQuery(' 770 '), '770');
});

test('record tabs split invoices by direction and follow read permissions', () => {
  assert.deepEqual(
    RECORD_TABS.map((t) => t.label),
    ['Gelen faturalar', 'Giden faturalar', 'Banka hareketleri', 'Z raporları', 'Masraflar / Fişler'],
  );
  assert.deepEqual(
    visibleRecordTabs(() => true).map((t) => t.key),
    ['incoming', 'outgoing', 'bank', 'zreports', 'expenses'],
  );
  assert.deepEqual(
    visibleRecordTabs((p) => p === 'tenant.invoices.read').map((t) => [t.key, t.kind, t.direction]),
    [
      ['incoming', 'invoices', 'incoming'],
      ['outgoing', 'invoices', 'outgoing'],
    ],
  );
  assert.deepEqual(visibleRecordTabs(() => false), []);
});

test('listTitle names the invoice direction; isDirection guards the route param', () => {
  assert.equal(listTitle('invoices', 'incoming'), 'Gelen faturalar');
  assert.equal(listTitle('invoices', 'outgoing'), 'Giden faturalar');
  assert.equal(listTitle('invoices', null), 'Faturalar');
  assert.equal(listTitle('bank', 'incoming'), 'Banka hareketleri');
  assert.equal(isDirection('incoming'), true);
  assert.equal(isDirection('in'), false);
  assert.equal(isDirection(undefined), false);
});

test('fişten oran okuma durumu ve çok oranlı masraf (#780)', () => {
  assert.deepEqual(receiptReadChip('read'), { label: 'Fişten okundu', tone: 'success' });
  assert.equal(receiptReadChip('mismatch')?.label, 'Fiş okunamadı / tutmadı');
  assert.equal(receiptReadChip('unreadable')?.tone, 'warning');
  assert.equal(receiptReadChip(null), null);
  assert.equal(vatRatesLabel([{ tax_rate: '1.00' }, { tax_rate: '20.00' }]), '%1 + %20');
  assert.equal(vatRatesLabel([{ tax_rate: '20.00' }]), '');
});

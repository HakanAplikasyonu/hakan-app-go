import { KristalYukleyici, YetkiYok } from '@hakan/mobile-core/hakan';
import { Stack, useLocalSearchParams } from 'expo-router';
import { useState } from 'react';
import { View } from 'react-native';

import { LoadError } from '@/components/load-error';
import { Button, Card, Chip, Notice, Screen, Text } from '@/components/ui';
import { useAuth, useOrganization } from '@/core/auth/auth-provider';
import { quantity } from '@/features/clients/format';
import { previewFile } from '@/features/files/open';
import {
  type BankTransaction,
  type Expense,
  type OfficeInvoiceDetail,
  type RetailSale,
  useMonthDetail,
  useMonthList,
} from '@/features/month/api';
import {
  amountLabel,
  approvalLabel,
  approvalTone,
  bankMatchLabel,
  dateLabel,
  isMonthKind,
  kindItemTitle,
  paymentMethodLabel,
  rateLabel,
  receiptReadChip,
  reviewSummary,
  type MonthKind,
} from '@/features/month/format';
import { InfoRow, ReceiptPhoto } from '@/features/month/parts';
import { ExpenseActions, ZReportActions } from '@/features/month/receipt-actions';
import { periodLabel } from '@/features/warnings/format';

const READ: Record<MonthKind, string> = {
  invoices: 'tenant.invoices.read',
  zreports: 'tenant.receipts.read',
  expenses: 'tenant.receipts.read',
  bank: 'tenant.bank.read',
};

/**
 * Belge ayrıntısı (TestFlight geri bildirimi, #780): fatura, Z raporu,
 * masraf ya da banka hareketi. Fiş görseli (varsa), okunan alanlar ve
 * "Kontrol et" işaretleri; bekleyen masraf ve Z raporunda onay.
 */
export default function MonthDocumentScreen() {
  const params = useLocalSearchParams<{ uuid: string; engagement: string; kind?: string; period?: string; title?: string }>();
  const kind: MonthKind = isMonthKind(params.kind) ? params.kind : 'invoices';
  const { can } = useAuth();
  const { uuid, engagement, title } = params;

  return (
    <Screen>
      <Stack.Screen options={{ title: kindItemTitle[kind] }} />
      {title ? (
        <Text variant="caption" tone="muted" numberOfLines={1}>
          {title}
        </Text>
      ) : null}
      {!can(READ[kind]) ? (
        <YetkiYok />
      ) : kind === 'invoices' ? (
        <InvoiceView engagement={engagement} uuid={uuid} />
      ) : kind === 'expenses' ? (
        <ExpenseView engagement={engagement} uuid={uuid} />
      ) : kind === 'zreports' ? (
        <ZReportView engagement={engagement} uuid={uuid} />
      ) : (
        <BankView engagement={engagement} uuid={uuid} period={params.period ?? ''} />
      )}
    </Screen>
  );
}

function InvoiceView({ engagement, uuid }: { engagement: string; uuid: string }) {
  const org = useOrganization();
  const q = useMonthDetail<OfficeInvoiceDetail>(engagement, 'invoices', uuid);
  const [pdfBusy, setPdfBusy] = useState(false);
  const [pdfError, setPdfError] = useState<string | null>(null);
  if (q.isLoading) return <KristalYukleyici />;
  if (!q.data) return <LoadError title="Fatura alınamadı" error={q.error} retry={q.refetch} retrying={q.isRefetching} />;
  const inv = q.data;
  const openWarnings = inv.warnings.filter((w) => w.status === 'open').length;

  const openPdf = async () => {
    setPdfBusy(true);
    setPdfError(null);
    try {
      await previewFile(org, {
        path: `/v1/tenant/engagements/${encodeURIComponent(engagement)}/invoices/${encodeURIComponent(inv.uuid)}/pdf`,
        uuid: inv.uuid,
        prefix: 'fatura',
        title: `Fatura ${inv.invoice_number}`,
        fileName: `${inv.invoice_number}.pdf`,
        contentType: 'application/pdf',
      });
    } catch {
      setPdfError('Fatura görüntüsü açılamadı. Bağlantınızı kontrol edip yeniden deneyin.');
    } finally {
      setPdfBusy(false);
    }
  };

  return (
    <>
      <Card>
        <Text variant="caption" tone="muted">
          {inv.direction === 'incoming' ? 'Gelen fatura' : 'Giden fatura'} · {inv.invoice_number}
        </Text>
        <Text variant="heading">{inv.counterparty_name || '—'}</Text>
        <View style={{ flexDirection: 'row', gap: 6, flexWrap: 'wrap' }}>
          {inv.status === 'cancelled' ? <Chip tone="danger" label="İptal" /> : null}
          {openWarnings ? <Chip tone="warning" label={`${openWarnings} açık uyarı`} /> : null}
        </View>
        <InfoRow label="Karşı taraf VKN/TCKN" value={inv.counterparty_tax_id} />
        <InfoRow label="Fatura tarihi" value={dateLabel(inv.issue_date)} />
        <InfoRow label="Dönem" value={periodLabel(inv.period)} />
        {inv.vat_deduction_period && inv.vat_deduction_period !== inv.period ? (
          <InfoRow label="KDV indirim dönemi" value={periodLabel(inv.vat_deduction_period)} />
        ) : null}
        <InfoRow label="Matrah" value={amountLabel(inv.tax_exclusive_amount, inv.currency)} />
        <InfoRow label="KDV" value={amountLabel(inv.tax_amount, inv.currency)} />
        {Number(inv.withholding_amount) ? <InfoRow label="Tevkifat" value={amountLabel(inv.withholding_amount, inv.currency)} /> : null}
        <InfoRow label="Ödenecek" value={amountLabel(inv.payable_amount, inv.currency)} />
        {inv.currency !== 'TRY' && inv.exchange_rate ? <InfoRow label="Kur" value={inv.exchange_rate} /> : null}
      </Card>

      {inv.has_ubl ? (
        <View style={{ gap: 6 }}>
          <Button testID="invoice-pdf" title="Faturayı görüntüle" variant="secondary" loading={pdfBusy} onPress={() => void openPdf()} />
          {pdfError ? <Notice>{pdfError}</Notice> : null}
        </View>
      ) : null}

      {inv.items.length ? (
        <Card>
          <Text variant="heading">Kalemler</Text>
          {inv.items.map((it) => (
            <View key={it.line_no} style={{ gap: 2 }}>
              <Text style={{ fontWeight: '600' }}>
                {it.line_no}. {it.name || '—'}
              </Text>
              <Text variant="caption" tone="muted">
                {quantity(it.quantity, it.unit_name, it.unit_code)} · {amountLabel(it.tax_exclusive_amount, inv.currency)} · KDV {rateLabel(it.tax_rate)}
              </Text>
            </View>
          ))}
        </Card>
      ) : null}

      {inv.tax_summary.length ? (
        <Card>
          <Text variant="heading">KDV dağılımı</Text>
          {inv.tax_summary.map((t) => (
            <InfoRow key={t.tax_rate} label={`KDV ${rateLabel(t.tax_rate)}`} value={`${amountLabel(t.base, inv.currency)} matrah · ${amountLabel(t.vat, inv.currency)} KDV`} />
          ))}
        </Card>
      ) : null}
    </>
  );
}

/** "Kontrol et" listesi: hem alanın yanında hem üstte özet. */
function ReviewNotice({ fields }: { fields: string[] }) {
  const text = reviewSummary(fields);
  if (!text) return null;
  return (
    <View testID="review-flag">
      <Notice>{`${text}. Belgeden okunamadı ya da emin olunamadı; görselle karşılaştırın.`}</Notice>
    </View>
  );
}

function ExpenseView({ engagement, uuid }: { engagement: string; uuid: string }) {
  const q = useMonthDetail<Expense>(engagement, 'expenses', uuid);
  if (q.isLoading) return <KristalYukleyici />;
  if (!q.data) return <LoadError title="Masraf alınamadı" error={q.error} retry={q.refetch} retrying={q.isRefetching} />;
  const e = q.data;
  const f = new Set(e.review_fields);
  return (
    <>
      {e.has_attachment ? <ReceiptPhoto engagement={engagement} draft={e.uuid} period={e.period} /> : null}
      <ReviewNotice fields={e.review_fields} />
      <Card>
        <View style={{ flexDirection: 'row', alignItems: 'center', gap: 8 }}>
          <Text variant="heading" style={{ flex: 1 }}>
            {e.vendor_name || e.description || 'Masraf'}
          </Text>
          <Chip tone={approvalTone[e.approval_status]} label={approvalLabel[e.approval_status]} />
        </View>
        <InfoRow label="Fiş tarihi" value={dateLabel(e.expense_date)} check={f.has('date')} />
        <InfoRow label="Satıcı adı" value={e.vendor_name} check={f.has('vendor_name')} />
        <InfoRow label="Satıcı VKN" value={e.vendor_tax_number} check={f.has('vendor_tax_number')} />
        <InfoRow label="Fiş no" value={e.receipt_number} check={f.has('receipt_number')} />
        <InfoRow label="Fiş toplamı" value={amountLabel(e.amount, e.currency)} check={f.has('total_amount')} />
        <InfoRow label="KDV tutarı" value={amountLabel(e.tax_amount, e.currency)} check={f.has('tax_amount')} />
        {e.vat_lines.length > 1 ? null : <InfoRow label="KDV oranı" value={rateLabel(e.tax_rate)} check={f.has('tax_rate')} />}
        {receiptReadChip(e.receipt_read) ? <InfoRow label="KDV oranları" value={receiptReadChip(e.receipt_read)?.label ?? null} check={e.receipt_read === 'mismatch' || e.receipt_read === 'unreadable'} /> : null}
        {e.vat_lines.map((l) => {
          const rate = String(Number(l.tax_rate));
          return (
            <InfoRow
              key={l.tax_rate}
              label={`KDV ${rateLabel(l.tax_rate)}`}
              value={`${amountLabel(l.tax_base, e.currency)} matrah · ${amountLabel(l.tax_amount, e.currency)} KDV`}
              check={f.has(`vat_${rate}_tax`) || f.has(`vat_${rate}_total`)}
            />
          );
        })}
        {e.vehicle_plate || f.has('vehicle_plate') ? <InfoRow label="Plaka" value={e.vehicle_plate} check={f.has('vehicle_plate')} /> : null}
        <InfoRow label="Ödeme şekli" value={paymentMethodLabel(e.payment_method)} check={f.has('payment_method')} />
        {e.description && e.description !== e.vendor_name ? <InfoRow label="Açıklama" value={e.description} /> : null}
        <InfoRow label="Dönem" value={periodLabel(e.period)} />
      </Card>
      <ExpenseActions engagement={engagement} expense={e} />
    </>
  );
}

function ZReportView({ engagement, uuid }: { engagement: string; uuid: string }) {
  const q = useMonthDetail<RetailSale>(engagement, 'zreports', uuid);
  if (q.isLoading) return <KristalYukleyici />;
  if (!q.data) return <LoadError title="Z raporu alınamadı" error={q.error} retry={q.refetch} retrying={q.isRefetching} />;
  const z = q.data;
  const f = new Set(z.review_fields);
  return (
    <>
      {z.has_attachment ? <ReceiptPhoto engagement={engagement} draft={z.uuid} period={z.period} /> : null}
      <ReviewNotice fields={z.review_fields} />
      {z.breakdown_mismatch ? <Notice>Nakit, kart ve diğer ödemelerin toplamı rapor toplamını tutmuyor.</Notice> : null}
      <Card>
        <View style={{ flexDirection: 'row', alignItems: 'center', gap: 8 }}>
          <Text variant="heading" style={{ flex: 1 }}>
            {z.z_report_number ? `Z raporu ${z.z_report_number}` : 'Z raporu'}
          </Text>
          <Chip tone={approvalTone[z.approval_status]} label={approvalLabel[z.approval_status]} />
        </View>
        <InfoRow label="Tarih" value={dateLabel(z.sale_date)} check={f.has('date')} />
        <InfoRow label="Z raporu no" value={z.z_report_number} check={f.has('z_report_number')} />
        <InfoRow label="ÖKC sicil no" value={z.okc_serial_number} check={f.has('okc_serial_number')} />
        <InfoRow label="Toplam" value={amountLabel(z.total_amount, z.currency)} check={f.has('total_amount')} />
        <InfoRow label="KDV tutarı" value={amountLabel(z.tax_amount, z.currency)} check={f.has('tax_amount')} />
        <InfoRow label="Nakit" value={amountLabel(z.cash_amount, z.currency)} check={f.has('cash_amount')} />
        <InfoRow label="Kart" value={amountLabel(z.card_amount, z.currency)} check={f.has('card_amount')} />
        {z.other_payment_amount && Number(z.other_payment_amount) ? <InfoRow label="Diğer ödeme" value={amountLabel(z.other_payment_amount, z.currency)} /> : null}
        <InfoRow label="Fiş sayısı" value={typeof z.receipt_count === 'number' ? String(z.receipt_count) : null} check={f.has('receipt_count')} />
        {z.vat_lines.map((l) => {
          const rate = String(Number(l.tax_rate));
          return (
            <InfoRow
              key={l.tax_rate}
              label={`KDV ${rateLabel(l.tax_rate)}`}
              value={`${amountLabel(l.tax_base, z.currency)} matrah · ${amountLabel(l.tax_amount, z.currency)} KDV`}
              check={f.has(`vat_${rate}_tax`) || f.has(`vat_${rate}_total`)}
            />
          );
        })}
        <InfoRow label="Dönem" value={periodLabel(z.period)} />
        {z.journal ? <InfoRow label="Yevmiye fişi" value={String(z.journal.entry_no)} /> : null}
      </Card>
      {z.approval_status === 'pending' ? <ZReportActions engagement={engagement} uuid={z.uuid} /> : null}
    </>
  );
}

/** Banka hareketinin ayrı ucu yok: seçili dönemin listesinden (önbellekten) okunur. */
function BankView({ engagement, uuid, period }: { engagement: string; uuid: string; period: string }) {
  const q = useMonthList(engagement, 'bank', period);
  if (q.isLoading) return <KristalYukleyici />;
  if (q.error && !q.data) return <LoadError title="Banka hareketi alınamadı" error={q.error} retry={q.refetch} retrying={q.isRefetching} />;
  const t = (q.data?.pages ?? []).flatMap((p) => p.items as BankTransaction[]).find((x) => x.uuid === uuid);
  if (!t) return <Notice>Banka hareketi bulunamadı; listeyi yenileyip yeniden açın.</Notice>;
  return (
    <Card>
      <View style={{ flexDirection: 'row', alignItems: 'center', gap: 8 }}>
        <Text variant="heading" style={{ flex: 1 }}>
          {t.counterparty_name || 'Banka hareketi'}
        </Text>
        <Chip tone={t.match_status === 'matched' || t.booked_account_code ? 'success' : 'warning'} label={bankMatchLabel(t)} />
      </View>
      <InfoRow label="Tarih" value={dateLabel(t.transaction_date)} />
      <InfoRow label="Tutar" value={`${t.direction === 'in' ? 'Giriş' : 'Çıkış'} · ${amountLabel(t.amount, t.currency)}`} />
      <InfoRow label="Hesap" value={t.cash_box_name} />
      <InfoRow label="Açıklama" value={t.description} />
      {t.booked_account_code ? <InfoRow label="İşlendiği hesap" value={`${t.booked_account_code}${t.booked_account_name ? ` · ${t.booked_account_name}` : ''}`} /> : null}
      {t.possible_duplicate ? <Chip tone="warning" label="Mükerrer olabilir" /> : null}
      <Text variant="caption" tone="muted">
        Eşleştirme ve hesaba işleme ofis panelindeki Banka sayfasından yapılır.
      </Text>
    </Card>
  );
}

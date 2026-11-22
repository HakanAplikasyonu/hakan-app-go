import { useMemo, useState } from 'react';
import { ActivityIndicator, StyleSheet, View } from 'react-native';

import { PeriodChips } from '@/components/period-chips';
import { Button, Card, Chip, Field, Notice, Text } from '@/components/ui';
import { errorMessage } from '@/core/api/envelope';
import { useAuth } from '@/core/auth/auth-provider';
import { useColors } from '@/core/theme';
import { recentPeriods } from '@/features/clients/format';

import { type ForeignInvoice, useForeignInvoiceAction, useForeignInvoices, useForeignInvoicesEnabled } from './api';
import { amountLabel, parseRate, rateLabel, rowSubtitle, sourceLabel, statusLabel, totalsSummary } from './format';

/**
 * Mükellef ayrıntısında "Yabancı faturalar / KDV2" (#780): yurt dışı hizmet
 * faturaları (Google, Meta, AWS…) KDV2 beyanına girer. Ofis müşaviri dönemi
 * seçer, toplamı görür, faturayı onaylar ya da eksik kuru girer. Ofis ayarı
 * kapalıysa (`enabled` false) bölüm hiç görünmez.
 */
export function ForeignInvoicesCard({ engagement }: { engagement: string }) {
  const { can } = useAuth();
  const c = useColors();
  const readable = can('tenant.tax_returns.read');
  const writable = can('tenant.tax_returns.write');
  const [current, previous] = useMemo(() => recentPeriods(), []);
  const [period, setPeriod] = useState(current);
  const setting = useForeignInvoicesEnabled(readable);
  const list = useForeignInvoices(engagement, period, readable && setting.data === true);
  const action = useForeignInvoiceAction(engagement);
  const [editing, setEditing] = useState<string | null>(null);
  const [rateText, setRateText] = useState('');
  const [rateError, setRateError] = useState<string | null>(null);

  if (!readable || setting.data !== true) return null;

  const d = list.data;
  const startRate = (uuid: string) => {
    setEditing(uuid);
    setRateText('');
    setRateError(null);
  };
  const saveRate = (uuid: string) => {
    const rate = parseRate(rateText);
    if (!rate) {
      setRateError('Kuru sayı olarak yazın, örn. 38,25');
      return;
    }
    action.mutate({ uuid, kind: 'rate', rate }, { onSuccess: () => setEditing(null) });
  };

  return (
    <Card>
      <View testID="client-foreign-invoices" style={{ gap: 4 }}>
        <Text variant="heading">Yabancı faturalar / KDV2</Text>
        <Text variant="caption" tone="muted">
          Yurt dışından alınan hizmet faturaları (Google, Meta, AWS gibi) KDV2 beyanına girer. Onayladığınız faturalar beyana alınır.
        </Text>
      </View>
      <PeriodChips
        periods={[
          { key: current, label: 'Bu ay' },
          { key: previous, label: 'Geçen ay' },
        ]}
        selected={period}
        onSelect={setPeriod}
        label={(p) => `${p.label} yabancı faturaları`}
        testIDPrefix="foreign-invoices-period"
      />
      {list.isLoading ? <ActivityIndicator color={c.brand} /> : null}
      {list.error ? <Notice>{errorMessage(list.error)}</Notice> : null}
      {action.error ? <Notice>{errorMessage(action.error)}</Notice> : null}
      {d ? (
        <View style={{ gap: 4 }}>
          <View style={styles.totals}>
            <Text style={{ flex: 1 }}>KDV2 toplamı</Text>
            <Text style={{ fontWeight: '600' }}>{amountLabel(d.totals.kdv2_try)}</Text>
          </View>
          <View style={styles.totals}>
            <Text style={{ flex: 1 }}>Stopaj toplamı</Text>
            <Text style={{ fontWeight: '600' }}>{amountLabel(d.totals.stopaj_try)}</Text>
          </View>
          <Text variant="caption" tone={d.totals.missing_rate > 0 ? 'danger' : 'muted'}>
            {totalsSummary(d.totals)}
          </Text>
        </View>
      ) : null}
      {d?.items.map((inv) => (
        <InvoiceRow
          key={inv.uuid}
          inv={inv}
          writable={writable}
          busy={action.isPending && action.variables?.uuid === inv.uuid}
          editing={editing === inv.uuid}
          rateText={rateText}
          rateError={rateError}
          onRateText={(t) => {
            setRateText(t);
            setRateError(null);
          }}
          onStartRate={() => startRate(inv.uuid)}
          onCancelRate={() => setEditing(null)}
          onSaveRate={() => saveRate(inv.uuid)}
          onConfirm={() => action.mutate({ uuid: inv.uuid, kind: 'confirm' })}
          onUnconfirm={() => action.mutate({ uuid: inv.uuid, kind: 'unconfirm' })}
        />
      ))}
    </Card>
  );
}

function InvoiceRow({
  inv,
  writable,
  busy,
  editing,
  rateText,
  rateError,
  onRateText,
  onStartRate,
  onCancelRate,
  onSaveRate,
  onConfirm,
  onUnconfirm,
}: {
  inv: ForeignInvoice;
  writable: boolean;
  busy: boolean;
  editing: boolean;
  rateText: string;
  rateError: string | null;
  onRateText: (t: string) => void;
  onStartRate: () => void;
  onCancelRate: () => void;
  onSaveRate: () => void;
  onConfirm: () => void;
  onUnconfirm: () => void;
}) {
  const c = useColors();
  const source = sourceLabel(inv.source_kind);
  const rate = rateLabel(inv);
  const confirmed = inv.status === 'confirmed';
  return (
    <View testID={`foreign-invoice-${inv.uuid}`} style={[styles.row, { borderColor: c.border }]}>
      <Text style={{ fontWeight: '600' }} numberOfLines={2}>
        {inv.vendor || 'Satıcı henüz okunmadı'}
      </Text>
      <Text variant="caption" tone="muted">
        {rowSubtitle(inv)}
      </Text>
      <View style={styles.chips}>
        <Chip tone={confirmed ? 'success' : 'neutral'} label={statusLabel(inv.status)} />
        {source ? <Chip tone="brand" label={source} /> : null}
        {rate ? <Chip tone={inv.missing_rate ? 'danger' : 'neutral'} label={rate} /> : null}
      </View>
      {writable && editing ? (
        <View style={{ gap: 8 }}>
          <Field
            label={`1 ${inv.currency} kaç TL?`}
            value={rateText}
            onChangeText={onRateText}
            keyboardType="decimal-pad"
            placeholder="Örn. 38,25"
            error={rateError}
            testID="foreign-invoice-rate-input"
          />
          <View style={styles.actions}>
            <View style={{ flex: 1 }}>
              <Button title="Kuru kaydet" onPress={onSaveRate} loading={busy} testID="foreign-invoice-rate-save" />
            </View>
            <View style={{ flex: 1 }}>
              <Button title="Vazgeç" variant="secondary" onPress={onCancelRate} disabled={busy} />
            </View>
          </View>
        </View>
      ) : writable ? (
        <View style={styles.actions}>
          {inv.missing_rate && !confirmed ? (
            <View style={{ flex: 1 }}>
              <Button title="Kur gir" variant="secondary" onPress={onStartRate} disabled={busy} testID="foreign-invoice-rate" />
            </View>
          ) : null}
          <View style={{ flex: 1 }}>
            {confirmed ? (
              <Button title="Onayı geri al" variant="secondary" onPress={onUnconfirm} loading={busy} testID="foreign-invoice-unconfirm" />
            ) : (
              <Button title="Onayla" onPress={onConfirm} loading={busy} disabled={inv.missing_rate} testID="foreign-invoice-confirm" />
            )}
          </View>
        </View>
      ) : null}
    </View>
  );
}

const styles = StyleSheet.create({
  totals: { flexDirection: 'row', alignItems: 'center', gap: 8 },
  row: { gap: 6, borderTopWidth: StyleSheet.hairlineWidth, paddingTop: 10 },
  chips: { flexDirection: 'row', flexWrap: 'wrap', gap: 6 },
  actions: { flexDirection: 'row', gap: 10 },
});

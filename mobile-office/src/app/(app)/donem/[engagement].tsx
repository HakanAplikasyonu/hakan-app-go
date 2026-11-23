import { BosDurum, KristalYukleyici, YetkiYok } from '@hakan/mobile-core/hakan';
import { Stack, useLocalSearchParams, useRouter } from 'expo-router';
import { useMemo, useState } from 'react';
import { View } from 'react-native';

import { LoadError } from '@/components/load-error';
import { PeriodChips } from '@/components/period-chips';
import { Button, Chip, Row, Screen, Text } from '@/components/ui';
import { useAuth } from '@/core/auth/auth-provider';
import { type MonthItem, useMonthList } from '@/features/month/api';
import {
  amountLabel,
  approvalLabel,
  approvalTone,
  bankMatchLabel,
  dateLabel,
  isMonthKind,
  isPeriod,
  kindListTitle,
  matchStatusTone,
  pastPeriods,
  shortPeriodLabel,
  type MonthKind,
} from '@/features/month/format';
import { periodLabel } from '@/features/warnings/format';

const READ: Record<MonthKind, string> = {
  invoices: 'tenant.invoices.read',
  zreports: 'tenant.receipts.read',
  expenses: 'tenant.receipts.read',
  bank: 'tenant.bank.read',
};

const EMPTY: Record<MonthKind, string> = {
  invoices: 'Bu ay için fatura yok.',
  zreports: 'Bu ay için Z raporu yok.',
  expenses: 'Bu ay için masraf yok.',
  bank: 'Bu ay için banka hareketi yok.',
};

/**
 * Mükellefin seçili dönemdeki faturaları, Z raporları, masrafları ya da banka
 * hareketleri (TestFlight geri bildirimi, #780). Satıra dokununca belge
 * ayrıntısı açılır; dönem buradan da değiştirilebilir.
 */
export default function MonthListScreen() {
  const params = useLocalSearchParams<{ engagement: string; kind?: string; period?: string; title?: string }>();
  const { engagement, title } = params;
  const kind: MonthKind = isMonthKind(params.kind) ? params.kind : 'invoices';
  const periods = useMemo(() => pastPeriods(12).map((p) => ({ key: p, label: shortPeriodLabel(p) })), []);
  const [period, setPeriod] = useState(isPeriod(params.period) ? params.period : periods[0].key);
  const { can } = useAuth();
  const router = useRouter();
  const allowed = can(READ[kind]);

  return (
    <Screen>
      <Stack.Screen options={{ title: kindListTitle[kind] }} />
      {title ? (
        <Text variant="caption" tone="muted" numberOfLines={1}>
          {title}
        </Text>
      ) : null}
      {!allowed ? (
        <YetkiYok />
      ) : (
        <>
          <PeriodChips
            periods={periods.some((p) => p.key === period) ? periods : [{ key: period, label: shortPeriodLabel(period) }, ...periods]}
            selected={period}
            onSelect={setPeriod}
            label={(p) => `${periodLabel(p.key)} dönemi`}
            testIDPrefix="month-list-period"
          />
          <MonthList
            key={`${kind}-${period}`}
            engagement={engagement}
            kind={kind}
            period={period}
            onOpen={(uuid) => router.push({ pathname: '/belge/[uuid]', params: { uuid, engagement, kind, period, title: title ?? '' } })}
          />
        </>
      )}
    </Screen>
  );
}

function MonthList({ engagement, kind, period, onOpen }: { engagement: string; kind: MonthKind; period: string; onOpen: (uuid: string) => void }) {
  const q = useMonthList(engagement, kind, period);
  if (q.isLoading) return <KristalYukleyici />;
  if (q.error && !q.data) return <LoadError title="Liste alınamadı" error={q.error} retry={q.refetch} retrying={q.isRefetching} />;
  const pages = q.data?.pages ?? [];
  const total = pages[0]?.total ?? 0;
  const items = pages.flatMap((p) => p.items.map((item) => ({ kind, item }) as unknown as MonthItem));
  if (items.length === 0) return <BosDurum baslik={EMPTY[kind]} aciklama={`${periodLabel(period)} dönemi için kayıt bulunamadı. Başka bir ay seçebilirsiniz.`} />;

  return (
    <View style={{ gap: 8 }}>
      <Text variant="caption" tone="muted">
        {periodLabel(period)} · {total} kayıt
      </Text>
      {items.map((x) => (
        <View key={x.item.uuid} testID="month-row">
          <ItemRow entry={x} onPress={() => onOpen(x.item.uuid)} />
        </View>
      ))}
      {q.hasNextPage ? (
        <Button title="Daha fazla göster" variant="secondary" loading={q.isFetchingNextPage} onPress={() => void q.fetchNextPage()} />
      ) : null}
    </View>
  );
}

function ItemRow({ entry, onPress }: { entry: MonthItem; onPress: () => void }) {
  switch (entry.kind) {
    case 'invoices': {
      const inv = entry.item;
      return (
        <Row
          title={inv.counterparty_name || inv.invoice_number}
          subtitle={`${inv.direction === 'incoming' ? 'Gelen' : 'Giden'} · ${dateLabel(inv.issue_date)} · ${amountLabel(inv.payable_amount, inv.currency)}`}
          trailing={
            <Chips
              items={[
                inv.status === 'cancelled' ? { label: 'İptal', tone: 'danger' as const } : null,
                inv.open_warnings ? { label: `${inv.open_warnings} uyarı`, tone: 'warning' as const } : null,
              ]}
            />
          }
          onPress={onPress}
        />
      );
    }
    case 'zreports': {
      const z = entry.item;
      return (
        <Row
          title={z.z_report_number ? `Z raporu ${z.z_report_number}` : 'Z raporu'}
          subtitle={`${dateLabel(z.sale_date)} · ${amountLabel(z.total_amount, z.currency)}${z.okc_serial_number ? ` · ÖKC ${z.okc_serial_number}` : ''}`}
          trailing={
            <Chips
              items={[
                { label: approvalLabel[z.approval_status], tone: approvalTone[z.approval_status] },
                z.review_fields.length ? { label: 'Kontrol et', tone: 'warning' as const } : null,
              ]}
            />
          }
          onPress={onPress}
        />
      );
    }
    case 'expenses': {
      const e = entry.item;
      return (
        <Row
          title={e.vendor_name || e.description || 'Masraf'}
          subtitle={`${dateLabel(e.expense_date)} · ${amountLabel(e.amount, e.currency)}${e.account_code ? ` · ${e.account_code}` : ''}`}
          trailing={
            <Chips
              items={[
                { label: approvalLabel[e.approval_status], tone: approvalTone[e.approval_status] },
                e.review_fields.length ? { label: 'Kontrol et', tone: 'warning' as const } : null,
              ]}
            />
          }
          onPress={onPress}
        />
      );
    }
    case 'bank': {
      const t = entry.item;
      return (
        <Row
          title={t.counterparty_name || t.description || 'Banka hareketi'}
          subtitle={`${dateLabel(t.transaction_date)} · ${t.direction === 'in' ? '+' : '−'}${amountLabel(t.amount, t.currency)} · ${t.cash_box_name}`}
          trailing={<Chips items={[{ label: bankMatchLabel(t), tone: t.booked_account_code && t.match_status === 'unmatched' ? 'success' : matchStatusTone[t.match_status] }]} />}
          onPress={onPress}
        />
      );
    }
  }
}

type ChipItem = { label: string; tone: 'neutral' | 'brand' | 'success' | 'warning' | 'danger' } | null;

function Chips({ items }: { items: ChipItem[] }) {
  const shown = items.filter((x): x is NonNullable<ChipItem> => !!x);
  if (!shown.length) return null;
  return (
    <View style={{ gap: 4, alignItems: 'flex-end' }}>
      {shown.map((x) => (
        <Chip key={x.label} label={x.label} tone={x.tone} />
      ))}
    </View>
  );
}

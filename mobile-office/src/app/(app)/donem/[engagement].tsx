import { BosDurum, KristalYukleyici, YetkiYok } from '@hakan/mobile-core/hakan';
import { Stack, useLocalSearchParams, useRouter } from 'expo-router';
import { useMemo, useState } from 'react';
import { View } from 'react-native';

import { LoadError } from '@/components/load-error';
import { PeriodChips } from '@/components/period-chips';
import { Button, Screen, Text } from '@/components/ui';
import { useAuth } from '@/core/auth/auth-provider';
import { type MonthItem, useMonthList } from '@/features/month/api';
import { isDirection, isMonthKind, isPeriod, listTitle, pastPeriods, shortPeriodLabel, type InvoiceDirection, type MonthKind } from '@/features/month/format';
import { ItemRow } from '@/features/month/item-row';
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
  const params = useLocalSearchParams<{ engagement: string; kind?: string; period?: string; title?: string; direction?: string }>();
  const { engagement, title } = params;
  const kind: MonthKind = isMonthKind(params.kind) ? params.kind : 'invoices';
  const direction: InvoiceDirection | null = kind === 'invoices' && isDirection(params.direction) ? params.direction : null;
  const periods = useMemo(() => pastPeriods(12).map((p) => ({ key: p, label: shortPeriodLabel(p) })), []);
  const [period, setPeriod] = useState(isPeriod(params.period) ? params.period : periods[0].key);
  const { can } = useAuth();
  const router = useRouter();
  const allowed = can(READ[kind]);

  return (
    <Screen>
      <Stack.Screen options={{ title: listTitle(kind, direction) }} />
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
            key={`${kind}-${direction ?? ''}-${period}`}
            engagement={engagement}
            kind={kind}
            direction={direction}
            period={period}
            onOpen={(uuid) => router.push({ pathname: '/belge/[uuid]', params: { uuid, engagement, kind, period, title: title ?? '' } })}
          />
        </>
      )}
    </Screen>
  );
}

function MonthList({
  engagement,
  kind,
  direction,
  period,
  onOpen,
}: {
  engagement: string;
  kind: MonthKind;
  direction: InvoiceDirection | null;
  period: string;
  onOpen: (uuid: string) => void;
}) {
  const q = useMonthList(engagement, kind, period, direction);
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

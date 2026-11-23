import { useRouter } from 'expo-router';
import { useMemo } from 'react';
import { ActivityIndicator, Pressable, StyleSheet, View } from 'react-native';

import { PeriodChips } from '@/components/period-chips';
import { Text } from '@/components/ui';
import { useAuth } from '@/core/auth/auth-provider';
import { useColors } from '@/core/theme';
import { periodLabel } from '@/features/warnings/format';

import { useMonthCounts } from './api';
import { bankStatus, invoiceStatus, kindTitle, pastPeriods, receiptStatus, shortPeriodLabel, type MonthKind } from './format';

type CardData = { kind: MonthKind; count: number | undefined; loading: boolean; failed: boolean; status: string; warn: boolean };

/**
 * Mükellef ayrıntısının üstü (TestFlight geri bildirimi, #780; web'deki
 * dört kartla aynı): dönem seçici ve dönemin Fatura, Z raporu, Masraf, Banka
 * kartları. Karta dokununca o ayın listesi açılır. Her kart kendi okuma
 * iznine bağlı.
 */
export function MonthCards({
  engagement,
  title,
  period,
  onPeriod,
}: {
  engagement: string;
  title: string;
  period: string;
  onPeriod: (period: string) => void;
}) {
  const { can } = useAuth();
  const router = useRouter();
  const periods = useMemo(() => pastPeriods(12).map((p) => ({ key: p, label: shortPeriodLabel(p) })), []);
  const allowed = { invoices: can('tenant.invoices.read'), receipts: can('tenant.receipts.read'), bank: can('tenant.bank.read') };
  const q = useMonthCounts(engagement, period, allowed);

  const cards: CardData[] = [];
  if (allowed.invoices) {
    const failed = q.invoices.isError || q.incoming.isError;
    cards.push({
      kind: 'invoices',
      count: q.invoices.data?.total,
      loading: q.invoices.isLoading,
      failed,
      status: invoiceStatus(q.invoices.data?.total, q.incoming.data?.total),
      warn: false,
    });
  }
  if (allowed.receipts) {
    for (const kind of ['zreports', 'expenses'] as const) {
      const r = kind === 'zreports' ? q.zreports : q.expenses;
      cards.push({
        kind,
        count: r.data?.total,
        loading: r.isLoading,
        failed: r.isError,
        status: receiptStatus(r.data),
        warn: (r.data?.pending ?? 0) > 0,
      });
    }
  }
  if (allowed.bank) {
    const s = q.bank.data?.summary;
    cards.push({
      kind: 'bank',
      count: s?.total,
      loading: q.bank.isLoading,
      failed: q.bank.isError,
      status: bankStatus(s),
      warn: !!s && s.unmatched + s.suggested > 0,
    });
  }
  if (!cards.length) return null;

  return (
    <View style={{ gap: 10 }} testID="month-summary">
      <Text variant="heading">{periodLabel(period)} özeti</Text>
      <PeriodChips
        periods={periods}
        selected={period}
        onSelect={onPeriod}
        label={(p) => `${periodLabel(p.key)} dönemi`}
        testIDPrefix="month-period"
      />
      <View style={styles.grid}>
        {cards.map((card) => (
          <MonthCard
            key={card.kind}
            card={card}
            onPress={() => router.push({ pathname: '/donem/[engagement]', params: { engagement, kind: card.kind, period, title } })}
          />
        ))}
      </View>
    </View>
  );
}

function MonthCard({ card, onPress }: { card: CardData; onPress: () => void }) {
  const c = useColors();
  const status = card.failed ? 'Yüklenemedi; yeniden deneyin' : card.status;
  return (
    <Pressable
      testID={`month-card-${card.kind}`}
      accessibilityRole="button"
      accessibilityLabel={`${kindTitle[card.kind]}: ${card.count ?? 'bilinmiyor'}. ${status}. Listeyi aç`}
      onPress={onPress}
      style={({ pressed }) => [styles.card, { backgroundColor: c.surface, borderColor: c.border, opacity: pressed ? 0.85 : 1 }]}>
      <Text variant="caption" tone="muted" style={{ fontWeight: '600' }}>
        {kindTitle[card.kind]}
      </Text>
      {card.loading ? (
        <ActivityIndicator color={c.brand} style={{ alignSelf: 'flex-start', minHeight: 34 }} />
      ) : (
        <Text variant="title">{card.failed ? '–' : (card.count ?? '–')}</Text>
      )}
      <Text
        variant="caption"
        tone={card.failed ? 'danger' : 'muted'}
        style={card.warn && !card.failed ? { color: c.warning, fontWeight: '600' } : undefined}
        numberOfLines={2}>
        {status}
      </Text>
    </Pressable>
  );
}

const styles = StyleSheet.create({
  grid: { flexDirection: 'row', flexWrap: 'wrap', gap: 10 },
  card: { flexBasis: '47%', flexGrow: 1, minHeight: 112, borderWidth: StyleSheet.hairlineWidth, borderRadius: 14, padding: 14, gap: 4 },
});

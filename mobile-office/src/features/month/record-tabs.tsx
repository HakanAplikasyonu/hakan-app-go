import { useRouter } from 'expo-router';
import { useMemo, useState } from 'react';
import { ActivityIndicator, Pressable, ScrollView, StyleSheet, View } from 'react-native';

import { PeriodChips } from '@/components/period-chips';
import { Button, Text } from '@/components/ui';
import { useAuth } from '@/core/auth/auth-provider';
import { useColors } from '@/core/theme';
import { periodLabel } from '@/features/warnings/format';

import { type MonthItem, useMonthList } from './api';
import { pastPeriods, shortPeriodLabel, visibleRecordTabs, type RecordTab, type RecordTabDef } from './format';
import { ItemRow } from './item-row';

const SHOWN = 8;

/**
 * Mükellef ayrıntısındaki kayıt sekmeleri (Oğuz'un canlı geri bildirimi):
 * Gelen faturalar, Giden faturalar, Banka hareketleri, Z raporları,
 * Masraflar / Fişler. Her sekme seçili ayın listesi; satıra dokununca belge
 * ayrıntısı (kalemler, KDV kırılımı, PDF / görsel) açılır. İlk satırlar
 * burada, tamamı "Tümünü gör" ile dönem ekranında.
 */
export function RecordTabs({ engagement, title, period, onPeriod }: { engagement: string; title: string; period: string; onPeriod: (p: string) => void }) {
  const { can } = useAuth();
  const c = useColors();
  const router = useRouter();
  const tabs = visibleRecordTabs(can);
  const [selected, setSelected] = useState<RecordTab | null>(null);
  const periods = useMemo(() => pastPeriods(12).map((p) => ({ key: p, label: shortPeriodLabel(p) })), []);
  const tab = tabs.find((t) => t.key === selected) ?? tabs[0];
  if (!tab) return null;

  return (
    <View style={{ gap: 10 }} testID="records">
      <Text variant="heading">Kayıtlar</Text>
      <ScrollView horizontal showsHorizontalScrollIndicator={false} contentContainerStyle={{ gap: 8 }} accessibilityRole="tablist">
        {tabs.map((t) => {
          const on = t.key === tab.key;
          return (
            <Pressable
              key={t.key}
              testID={`records-tab-${t.key}`}
              accessibilityRole="tab"
              accessibilityState={{ selected: on }}
              accessibilityLabel={`${t.label} sekmesi`}
              onPress={() => setSelected(t.key)}
              style={[styles.tab, { borderColor: on ? c.text : c.border, backgroundColor: on ? c.text : c.surface }]}>
              <Text variant="caption" style={{ color: on ? c.background : c.text, fontWeight: '600' }}>
                {t.label}
              </Text>
            </Pressable>
          );
        })}
      </ScrollView>
      <PeriodChips
        periods={periods.some((p) => p.key === period) ? periods : [{ key: period, label: shortPeriodLabel(period) }, ...periods]}
        selected={period}
        onSelect={onPeriod}
        label={(p) => `${periodLabel(p.key)} dönemi`}
        testIDPrefix="records-period"
      />
      <TabList
        key={`${tab.key}-${period}`}
        engagement={engagement}
        tab={tab}
        period={period}
        onOpen={(uuid) => router.push({ pathname: '/belge/[uuid]', params: { uuid, engagement, kind: tab.kind, period, title } })}
        onAll={() =>
          router.push({ pathname: '/donem/[engagement]', params: { engagement, kind: tab.kind, period, title, direction: tab.direction ?? '' } })
        }
      />
    </View>
  );
}

function TabList({ engagement, tab, period, onOpen, onAll }: { engagement: string; tab: RecordTabDef; period: string; onOpen: (uuid: string) => void; onAll: () => void }) {
  const c = useColors();
  const q = useMonthList(engagement, tab.kind, period, tab.direction);
  if (q.isLoading) return <ActivityIndicator color={c.brand} />;
  if (q.error && !q.data) {
    return (
      <View style={{ gap: 6 }}>
        <Text tone="danger" variant="caption">
          Liste alınamadı. Bağlantınızı kontrol edip yeniden deneyin.
        </Text>
        <Button title="Yeniden dene" variant="secondary" onPress={() => void q.refetch()} loading={q.isRefetching} />
      </View>
    );
  }
  const total = q.data?.pages[0]?.total ?? 0;
  const items = (q.data?.pages ?? []).flatMap((p) => p.items.map((item) => ({ kind: tab.kind, item }) as unknown as MonthItem));
  if (!items.length) {
    return (
      <Text tone="muted" testID="records-empty">
        {periodLabel(period)} döneminde {tab.label.toLocaleLowerCase('tr-TR')} için kayıt yok. Başka bir ay seçebilirsiniz.
      </Text>
    );
  }
  return (
    <View style={{ gap: 8 }} testID="records-list">
      <Text variant="caption" tone="muted">
        {periodLabel(period)} · {total} kayıt
      </Text>
      {items.slice(0, SHOWN).map((x) => (
        <View key={x.item.uuid} testID="records-row">
          <ItemRow entry={x} onPress={() => onOpen(x.item.uuid)} />
        </View>
      ))}
      {total > SHOWN ? <Button title={`Tümünü gör (${total})`} variant="secondary" onPress={onAll} /> : null}
    </View>
  );
}

const styles = StyleSheet.create({
  tab: { minHeight: 36, borderRadius: 10, borderWidth: 1, paddingHorizontal: 12, justifyContent: 'center' },
});

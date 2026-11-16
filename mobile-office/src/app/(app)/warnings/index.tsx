import { useLocalSearchParams, useRouter } from 'expo-router';
import { useMemo, useState } from 'react';
import { ActivityIndicator, FlatList, Pressable, RefreshControl, StyleSheet, View } from 'react-native';

import { Chip, Notice, Row, Text } from '@/components/ui';
import { errorMessage } from '@/core/api/envelope';
import { useColors } from '@/core/theme';
import { useWarnings } from '@/features/warnings/api';
import { periodLabel, severityLabel, severityTone, statusLabel, warningMessage, type WarningStatus } from '@/features/warnings/format';

const filters: { key: WarningStatus | 'all'; label: string }[] = [
  { key: 'open', label: 'Açık' },
  { key: 'acknowledged', label: 'Görüldü' },
  { key: 'resolved', label: 'Çözüldü' },
  { key: 'all', label: 'Tümü' },
];

export default function WarningsScreen() {
  const c = useColors();
  const router = useRouter();
  const { engagement } = useLocalSearchParams<{ engagement?: string }>();
  const [status, setStatus] = useState<WarningStatus | 'all'>('open');
  const [scoped, setScoped] = useState(!!engagement);
  const q = useWarnings(status, scoped ? engagement : undefined);
  const items = useMemo(() => q.data?.pages.flatMap((p) => p.items) ?? [], [q.data]);
  const total = q.data?.pages[0]?.total ?? 0;

  return (
    <View style={{ flex: 1, backgroundColor: c.background }}>
      <View style={styles.filters} accessibilityRole="tablist">
        {filters.map((f) => (
          <Pressable
            key={f.key}
            accessibilityRole="tab"
            accessibilityState={{ selected: status === f.key }}
            onPress={() => setStatus(f.key)}
            style={[styles.filter, { borderColor: status === f.key ? c.brand : c.border, backgroundColor: c.surface }]}>
            <Text variant="caption" tone={status === f.key ? 'brand' : 'muted'} style={{ fontWeight: '600' }}>
              {f.label}
            </Text>
          </Pressable>
        ))}
      </View>
      {q.error ? (
        <View style={{ padding: 16 }}>
          <Notice>{errorMessage(q.error)}</Notice>
        </View>
      ) : null}
      <FlatList
        data={items}
        keyExtractor={(w) => w.uuid}
        contentContainerStyle={{ padding: 16, gap: 8 }}
        refreshControl={<RefreshControl refreshing={q.isRefetching && !q.isFetchingNextPage} onRefresh={() => void q.refetch()} tintColor={c.brand} />}
        onEndReached={() => q.hasNextPage && !q.isFetchingNextPage && q.fetchNextPage()}
        onEndReachedThreshold={0.4}
        ListHeaderComponent={
          q.data ? (
            <View style={{ flexDirection: 'row', alignItems: 'center', gap: 8 }}>
              <Text variant="caption" tone="muted" style={{ flex: 1 }}>
                {total} uyarı{scoped ? ' · tek mükellef' : ''}
              </Text>
              {scoped ? (
                <Pressable accessibilityRole="button" onPress={() => setScoped(false)}>
                  <Text variant="caption" tone="brand">
                    Tüm mükellefler
                  </Text>
                </Pressable>
              ) : null}
            </View>
          ) : undefined
        }
        ListEmptyComponent={q.isLoading ? <ActivityIndicator color={c.brand} /> : <Text tone="muted">Bu filtreye uyan uyarı yok.</Text>}
        ListFooterComponent={q.isFetchingNextPage ? <ActivityIndicator color={c.brand} /> : undefined}
        renderItem={({ item: w }) => (
          <Row
            title={warningMessage(w.code, w.params)}
            subtitle={[periodLabel(w.period), statusLabel[w.status]].filter(Boolean).join(' · ')}
            leading={<Chip tone={severityTone(w.severity)} label={severityLabel[w.severity]} />}
            trailing={<Text tone="muted">›</Text>}
            onPress={() => router.push({ pathname: '/warnings/[uuid]', params: { uuid: w.uuid } })}
          />
        )}
      />
    </View>
  );
}

const styles = StyleSheet.create({
  filters: { flexDirection: 'row', gap: 8, paddingHorizontal: 16, paddingTop: 12, flexWrap: 'wrap' },
  filter: { borderWidth: 1, borderRadius: 999, paddingHorizontal: 12, paddingVertical: 6 },
});

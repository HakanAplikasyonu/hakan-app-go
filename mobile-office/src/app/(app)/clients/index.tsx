import { useRouter } from 'expo-router';
import { useEffect, useMemo, useState } from 'react';
import { ActivityIndicator, FlatList, Pressable, RefreshControl, StyleSheet, TextInput, View } from 'react-native';

import { Notice, Text } from '@/components/ui';
import { errorMessage } from '@/core/api/envelope';
import { useColors } from '@/core/theme';
import { useClients, type OfficeClient } from '@/features/clients/api';
import { clientSubtitle, filterLabels, normalizeSearch, type ClientFilter } from '@/features/clients/format';

const ROW_HEIGHT = 68;

/** Mükellef listesi (#238): arama, süzgeç, imleçli sonsuz kaydırma. */
export default function ClientsScreen() {
  const c = useColors();
  const router = useRouter();
  const [text, setText] = useState('');
  const [search, setSearch] = useState<string | undefined>();
  const [filter, setFilter] = useState<ClientFilter>('all');

  useEffect(() => {
    const t = setTimeout(() => setSearch(normalizeSearch(text)), 300);
    return () => clearTimeout(t);
  }, [text]);

  const q = useClients(search, filter);
  const items = useMemo(() => q.data?.pages.flatMap((p) => p.items) ?? [], [q.data]);

  return (
    <View style={{ flex: 1, backgroundColor: c.background }}>
      <View style={{ paddingHorizontal: 16, paddingTop: 12, gap: 10 }}>
        <TextInput
          testID="clients-search"
          value={text}
          onChangeText={setText}
          placeholder="Unvan ya da VKN/TCKN ara"
          placeholderTextColor={c.muted}
          autoCorrect={false}
          clearButtonMode="while-editing"
          accessibilityLabel="Mükellef ara"
          style={[styles.search, { color: c.text, backgroundColor: c.surface, borderColor: c.border }]}
        />
        <View style={styles.filters} accessibilityRole="tablist">
          {filterLabels.map((f) => (
            <Pressable
              key={f.key}
              accessibilityRole="tab"
              accessibilityState={{ selected: filter === f.key }}
              onPress={() => setFilter(f.key)}
              style={[styles.filter, { borderColor: filter === f.key ? c.brand : c.border, backgroundColor: c.surface }]}>
              <Text variant="caption" tone={filter === f.key ? 'brand' : 'muted'} style={{ fontWeight: '600' }}>
                {f.label}
              </Text>
            </Pressable>
          ))}
        </View>
      </View>
      {q.error ? (
        <View style={{ padding: 16 }}>
          <Notice>{errorMessage(q.error)}</Notice>
        </View>
      ) : null}
      <FlatList
        data={items}
        keyExtractor={(x) => x.engagement_uuid}
        contentContainerStyle={{ padding: 16 }}
        getItemLayout={(_, index) => ({ length: ROW_HEIGHT, offset: ROW_HEIGHT * index, index })}
        initialNumToRender={15}
        maxToRenderPerBatch={20}
        windowSize={11}
        removeClippedSubviews
        keyboardDismissMode="on-drag"
        refreshControl={<RefreshControl refreshing={q.isRefetching && !q.isFetchingNextPage} onRefresh={() => void q.refetch()} tintColor={c.brand} />}
        onEndReached={() => q.hasNextPage && !q.isFetchingNextPage && q.fetchNextPage()}
        onEndReachedThreshold={0.5}
        ListEmptyComponent={q.isLoading ? <ActivityIndicator color={c.brand} /> : <Text tone="muted">Mükellef bulunamadı.</Text>}
        ListFooterComponent={q.isFetchingNextPage ? <ActivityIndicator color={c.brand} /> : undefined}
        renderItem={({ item }) => (
          <ClientRow item={item} onPress={() => router.push({ pathname: '/clients/[engagement]', params: { engagement: item.engagement_uuid, title: item.title } })} />
        )}
      />
    </View>
  );
}

function ClientRow({ item, onPress }: { item: OfficeClient; onPress: () => void }) {
  const c = useColors();
  const dot = item.open_blockers ? c.danger : item.open_warnings ? c.warning : item.open_tasks ? c.brand : null;
  return (
    <Pressable
      accessibilityRole="button"
      accessibilityLabel={`${item.title}, ${clientSubtitle(item)}`}
      onPress={onPress}
      style={({ pressed }) => [styles.row, { borderColor: c.border, opacity: pressed ? 0.7 : 1 }]}>
      <View style={{ flex: 1 }}>
        <Text style={{ fontWeight: '600' }} numberOfLines={1}>
          {item.title}
        </Text>
        <Text variant="caption" tone="muted" numberOfLines={1}>
          {clientSubtitle(item)}
        </Text>
      </View>
      {dot ? <View style={[styles.dot, { backgroundColor: dot }]} /> : null}
      <Text tone="muted">›</Text>
    </Pressable>
  );
}

const styles = StyleSheet.create({
  search: { minHeight: 44, borderWidth: 1, borderRadius: 12, paddingHorizontal: 14, fontSize: 16 },
  filters: { flexDirection: 'row', gap: 8, flexWrap: 'wrap' },
  filter: { borderWidth: 1, borderRadius: 999, paddingHorizontal: 12, paddingVertical: 6 },
  row: { height: ROW_HEIGHT, flexDirection: 'row', alignItems: 'center', gap: 10, borderBottomWidth: StyleSheet.hairlineWidth },
  dot: { width: 10, height: 10, borderRadius: 5 },
});

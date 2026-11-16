import { inboxA11yLabel, inboxTime, type InboxItem } from '@hakan/mobile-core/notifications/inbox';
import { useRouter, type Href } from 'expo-router';
import { useState } from 'react';
import { FlatList, Pressable, RefreshControl, StyleSheet, View } from 'react-native';
import { SafeAreaView } from 'react-native-safe-area-context';

import { Card, Notice, Text } from '@/components/ui';
import { errorMessage } from '@/core/api/envelope';
import { hrefOf, resolveLink } from '@/core/push/links';
import { useColors } from '@/core/theme';
import { useInbox, useMarkRead } from '@/features/notifications/api';
import { filterInbox, inboxFilters, type InboxFilter } from '@/features/notifications/filter';

/**
 * Bildirimler (MusavirMobil · 5, yeni ekran): Tümü / Engel / Yükleme.
 * Liste ucu #333 ile gelecek; o zamana kadar boş durum gösterilir.
 */
export default function NotificationsScreen() {
  const c = useColors();
  const router = useRouter();
  const inbox = useInbox();
  const mark = useMarkRead();
  const [filter, setFilter] = useState<InboxFilter>('all');
  const items = filterInbox(inbox.items, filter);

  function open(i: InboxItem) {
    if (!i.read) mark.mutate(i.uuid);
    const t = i.url ? resolveLink(i.url) : null;
    if (t) router.push(hrefOf(t) as Href);
  }

  return (
    <SafeAreaView style={{ flex: 1, backgroundColor: c.background }} edges={['top', 'left', 'right']}>
      <View style={styles.head}>
        <Text variant="title" accessibilityRole="header">
          Bildirimler
        </Text>
      </View>
      <View accessibilityRole="tablist" style={[styles.segment, { backgroundColor: c.border }]}>
        {inboxFilters.map((f) => (
          <Pressable
            key={f.key}
            testID={`inbox-filter-${f.key}`}
            accessibilityRole="tab"
            accessibilityState={{ selected: filter === f.key }}
            onPress={() => setFilter(f.key)}
            style={[styles.seg, filter === f.key ? { backgroundColor: c.surface } : null]}>
            <Text variant="caption" style={{ fontWeight: '600' }}>
              {f.label}
            </Text>
          </Pressable>
        ))}
      </View>
      {inbox.error ? <Notice>{errorMessage(inbox.error)}</Notice> : null}
      <FlatList
        testID="inbox-list"
        data={items}
        keyExtractor={(i) => i.uuid}
        refreshControl={inbox.available ? <RefreshControl refreshing={inbox.isRefetching} onRefresh={() => void inbox.refetch()} /> : undefined}
        ListEmptyComponent={
          <Card style={{ margin: 16 }}>
            <Text variant="heading">{inbox.available ? 'Bildirim yok' : 'Bildirim listesi yakında'}</Text>
            <Text tone="muted">
              {inbox.available
                ? 'Mükellef belge yüklediğinde ve engelleyici uyarı açıldığında burada görürsünüz.'
                : 'Bildirimler şimdilik yalnızca telefonunuza anlık olarak gelir. Geçmiş bildirimlerin listesi bir sonraki sürümde burada olacak.'}
            </Text>
          </Card>
        }
        renderItem={({ item }) => (
          <Pressable
            testID="inbox-row"
            accessibilityRole="button"
            accessibilityLabel={inboxA11yLabel(item)}
            onPress={() => open(item)}
            style={[styles.row, { borderTopColor: c.border, backgroundColor: item.read ? 'transparent' : c.surface }]}>
            <View style={{ flex: 1, gap: 2 }}>
              <Text style={{ fontWeight: '600' }}>{item.title}</Text>
              {item.body ? (
                <Text variant="caption" tone="muted">
                  {item.body}
                </Text>
              ) : null}
            </View>
            <Text variant="caption" tone="muted">
              {inboxTime(item.createdAt)}
            </Text>
          </Pressable>
        )}
      />
    </SafeAreaView>
  );
}

const styles = StyleSheet.create({
  head: { paddingHorizontal: 20, paddingTop: 12, paddingBottom: 14 },
  segment: { flexDirection: 'row', gap: 2, padding: 3, borderRadius: 10, marginHorizontal: 16, marginBottom: 10 },
  seg: { flex: 1, minHeight: 44, borderRadius: 8, alignItems: 'center', justifyContent: 'center' },
  row: { flexDirection: 'row', alignItems: 'center', gap: 12, paddingHorizontal: 20, paddingVertical: 14, borderTopWidth: StyleSheet.hairlineWidth, minHeight: 44 },
});

import { crownFor, inboxA11yLabel, inboxTime, type InboxItem } from '@hakan/mobile-core/notifications/inbox';
import { BosDurum, KristalYukleyici, TacRozeti } from '@hakan/mobile-core/hakan';
import { EkranBasligi, metin, Satir, Segment } from '@hakan/mobile-core/ui/parcalar';
import { useTheme } from '@hakan/mobile-core/ui/theme';
import { useRouter, type Href } from 'expo-router';
import { useState } from 'react';
import { FlatList, RefreshControl, Text, View } from 'react-native';
import { SafeAreaView } from 'react-native-safe-area-context';

import { Notice } from '@/components/ui';
import { errorMessage } from '@/core/api/envelope';
import { hrefOf, resolveLink } from '@/core/push/links';
import { useInbox, useMarkRead } from '@/features/notifications/api';
import { filterInbox, inboxFilters, type InboxFilter } from '@/features/notifications/filter';

/**
 * Bildirimler (MusavirMobil · 5, yeni ekran): Tümü / Engel / Yükleme.
 * Okunmamış satır `brand-tint` zeminli, ekran okuyucu "Okunmadı" der.
 * Liste `GET /v1/tenant/notifications` (#333); okununca `{uuid}/read`.
 */
export default function NotificationsScreen() {
  const { colors: c } = useTheme();
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
    <SafeAreaView style={{ flex: 1, backgroundColor: c.bg }} edges={['top', 'left', 'right']}>
      <EkranBasligi baslik="Bildirimler" />
      <Segment testIDOnek="inbox-filter" secenekler={inboxFilters} secili={filter} onSec={setFilter} style={{ marginHorizontal: 16, marginBottom: 10 }} />
      {inbox.error ? (
        <View style={{ paddingHorizontal: 16 }}>
          <Notice>{errorMessage(inbox.error)}</Notice>
        </View>
      ) : null}
      <FlatList
        testID="inbox-list"
        data={items}
        keyExtractor={(i) => i.uuid}
        refreshControl={
          inbox.available ? <RefreshControl refreshing={inbox.isRefetching} onRefresh={() => void inbox.refetch()} tintColor={c.brandText} /> : undefined
        }
        ListEmptyComponent={
          inbox.isLoading ? (
            <View style={{ alignItems: 'center', paddingVertical: 40 }}>
              <KristalYukleyici etiket="Bildirimler yükleniyor" />
            </View>
          ) : (
            <BosDurum
              baslik={inbox.available ? 'Bildirim yok' : 'Bildirim listesi yakında'}
              aciklama={
                inbox.available
                  ? 'Mükellef belge yüklediğinde ve engelleyici uyarı açıldığında burada görürsünüz.'
                  : 'Bildirimler şimdilik telefonunuza anlık olarak gelir. Geçmiş bildirimlerin listesi bir sonraki sürümde burada olacak.'
              }
            />
          )
        }
        renderItem={({ item, index }) => (
          <Satir
            testID="inbox-row"
            ilk={index === 0}
            vurgulu={!item.read}
            sol={<TacRozeti durum={crownFor(item.kind)} />}
            baslik={item.title}
            alt={item.body || undefined}
            sag={<Text style={[metin.xs, { color: c.text3 }]}>{inboxTime(item.createdAt)}</Text>}
            accessibilityLabel={inboxA11yLabel(item)}
            onPress={() => open(item)}
          />
        )}
      />
    </SafeAreaView>
  );
}

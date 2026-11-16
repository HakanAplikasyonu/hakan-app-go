import { crownFor, inboxA11yLabel, inboxTime, unreadCount, type InboxItem } from '@hakan/mobile-core/notifications/inbox';
import { BosDurum, KristalYukleyici, TacRozeti } from '@hakan/mobile-core/hakan';
import { DOKUNMA } from '@hakan/mobile-core/ui/layout';
import { metin, Satir } from '@hakan/mobile-core/ui/parcalar';
import { useTheme } from '@hakan/mobile-core/ui/theme';
import { router, Stack, type Href } from 'expo-router';
import { FlatList, Pressable, RefreshControl, Text, View } from 'react-native';
import { SafeAreaView } from 'react-native-safe-area-context';

import { Notice } from '@/components/ui';
import { errorMessage } from '@/core/api/envelope';
import { resolveLink } from '@/core/push/links';
import { useInbox, useMarkRead } from '@/features/notifications/api';

/**
 * Bildirimler (MukellefMobil · 4, yeni ekran): okunmamış satır vurgulu,
 * tür ikonu olarak taç; "Tümü okundu". Liste `GET /v1/taxpayer/notifications`
 * (#333); okununca `{uuid}/read`, hepsi `read-all`.
 */
export default function NotificationsScreen() {
  const { colors: c } = useTheme();
  const inbox = useInbox();
  const mark = useMarkRead();
  const unread = unreadCount(inbox.items);

  function open(i: InboxItem) {
    if (!i.read) mark.mutate(i.uuid);
    const t = i.url ? resolveLink(i.url) : null;
    if (t) router.push(t.route as Href);
  }

  return (
    <SafeAreaView style={{ flex: 1, backgroundColor: c.bg }} edges={['left', 'right', 'bottom']}>
      <Stack.Screen
        options={{
          headerRight: () =>
            unread ? (
              <Pressable
                testID="inbox-read-all"
                accessibilityRole="button"
                onPress={() => mark.mutate('all')}
                style={{ minHeight: DOKUNMA, justifyContent: 'center', paddingHorizontal: 8 }}>
                <Text style={[metin.sm, { fontFamily: 'Inter_600SemiBold', color: c.brandText }]}>Tümü okundu</Text>
              </Pressable>
            ) : null,
        }}
      />
      {inbox.error ? (
        <View style={{ padding: 16 }}>
          <Notice>{errorMessage(inbox.error)}</Notice>
        </View>
      ) : null}
      <FlatList
        testID="inbox-list"
        data={inbox.items}
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
                  ? 'Müşaviriniz belge istediğinde ya da fişleriniz onaylandığında burada görürsünüz.'
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

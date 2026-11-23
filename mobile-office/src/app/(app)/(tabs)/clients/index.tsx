import { BosDurum, KristalYukleyici, TacRozeti, tacEtiketleri } from '@hakan/mobile-core/hakan';
import { Ikon } from '@hakan/mobile-core/ui/ikon';
import { DOKUNMA } from '@hakan/mobile-core/ui/layout';
import { EkranBasligi, metin } from '@hakan/mobile-core/ui/parcalar';
import { useTheme } from '@hakan/mobile-core/ui/theme';
import { useRouter } from 'expo-router';
import { useEffect, useMemo, useState } from 'react';
import { FlatList, Pressable, RefreshControl, ScrollView, StyleSheet, Text, TextInput, View } from 'react-native';
import { SafeAreaView } from 'react-native-safe-area-context';

import { Notice } from '@/components/ui';
import { useAuth } from '@/core/auth/auth-provider';
import { errorMessage } from '@/core/api/envelope';
import { useClients, type OfficeClient } from '@/features/clients/api';
import { clientCrown, clientSubtitle, filterLabels, initials, normalizeSearch, type ClientFilter } from '@/features/clients/format';

const ROW_HEIGHT = 64;

/** Mükellefler (MusavirMobil · 3): arama, süzgeç, satırda taç durumu; imleçli sonsuz kaydırma (#238). */
export default function ClientsScreen() {
  const { colors: c } = useTheme();
  const router = useRouter();
  const { can } = useAuth();
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
    <SafeAreaView style={{ flex: 1, backgroundColor: c.bg }} edges={['top', 'left', 'right']}>
      <EkranBasligi
        baslik="Mükellefler"
        sag={
          can('tenant.engagements.write') ? (
            <Pressable
              testID="clients-add"
              accessibilityRole="button"
              accessibilityLabel="Mükellef ekle"
              hitSlop={8}
              onPress={() => router.push('/mukellef-ekle')}
              style={[styles.add, { backgroundColor: c.brandTint }]}>
              <Text style={[metin.sm, { fontFamily: 'Inter_600SemiBold', color: c.brandText }]}>+ Ekle</Text>
            </Pressable>
          ) : undefined
        }
      />
      <View style={{ paddingHorizontal: 16, gap: 10, paddingBottom: 10 }}>
        <View style={[styles.search, { backgroundColor: c.surface, borderColor: c.line }]}>
          <Ikon ad="ara" boyut={16} renk={c.text3} />
          <TextInput
            testID="clients-search"
            value={text}
            onChangeText={setText}
            placeholder="Unvan, VKN, TCKN"
            placeholderTextColor={c.text3}
            autoCorrect={false}
            clearButtonMode="while-editing"
            accessibilityLabel="Mükellef ara"
            style={[metin.govde, { flex: 1, color: c.text, minHeight: DOKUNMA }]}
          />
        </View>
        <ScrollView horizontal showsHorizontalScrollIndicator={false} contentContainerStyle={{ gap: 6 }} accessibilityRole="tablist">
          {filterLabels.map((f) => {
            const on = filter === f.key;
            return (
              <Pressable
                key={f.key}
                accessibilityRole="tab"
                accessibilityState={{ selected: on }}
                hitSlop={{ top: 6, bottom: 6 }}
                onPress={() => setFilter(f.key)}
                style={[styles.chip, { backgroundColor: on ? c.brandTint : c.surface3 }]}>
                <Text style={[metin.xs, { fontFamily: 'Inter_600SemiBold', color: on ? c.brandText : c.text2 }]}>{f.label}</Text>
              </Pressable>
            );
          })}
        </ScrollView>
      </View>
      {q.error ? (
        <View style={{ padding: 16 }}>
          <Notice>{errorMessage(q.error)}</Notice>
        </View>
      ) : null}
      <FlatList
        data={items}
        keyExtractor={(x) => x.engagement_uuid}
        getItemLayout={(_, index) => ({ length: ROW_HEIGHT, offset: ROW_HEIGHT * index, index })}
        initialNumToRender={15}
        maxToRenderPerBatch={20}
        windowSize={11}
        removeClippedSubviews
        keyboardDismissMode="on-drag"
        refreshControl={<RefreshControl refreshing={q.isRefetching && !q.isFetchingNextPage} onRefresh={() => void q.refetch()} tintColor={c.brandText} />}
        onEndReached={() => q.hasNextPage && !q.isFetchingNextPage && q.fetchNextPage()}
        onEndReachedThreshold={0.5}
        ListEmptyComponent={
          q.isLoading ? (
            <View style={{ alignItems: 'center', paddingVertical: 40 }}>
              <KristalYukleyici etiket="Mükellefler yükleniyor" />
            </View>
          ) : (
            <BosDurum baslik="Mükellef bulunamadı" aciklama={search ? 'Aramayı değiştirip yeniden deneyin.' : 'Size atanmış mükellef yok.'} />
          )
        }
        ListFooterComponent={q.isFetchingNextPage ? <KristalYukleyici boyut={24} etiket="Devamı yükleniyor" /> : undefined}
        renderItem={({ item }) => (
          <ClientRow item={item} onPress={() => router.push({ pathname: '/clients/[engagement]', params: { engagement: item.engagement_uuid, title: item.title } })} />
        )}
      />
    </SafeAreaView>
  );
}

function ClientRow({ item, onPress }: { item: OfficeClient; onPress: () => void }) {
  const { colors: c } = useTheme();
  const tac = clientCrown(item);
  return (
    <Pressable
      testID="client-row"
      accessibilityRole="button"
      accessibilityLabel={`${item.title}, ${clientSubtitle(item)}, ${tacEtiketleri[tac]}`}
      onPress={onPress}
      style={({ pressed }) => [styles.row, { borderTopColor: c.line, backgroundColor: pressed ? c.surface2 : 'transparent' }]}>
      <View style={[styles.av, { backgroundColor: c.surface3 }]}>
        <Text style={[metin.xs, { fontFamily: 'Inter_700Bold', color: c.text2 }]}>{initials(item.title)}</Text>
      </View>
      <View style={{ flex: 1 }}>
        <Text style={[metin.sm, { fontFamily: 'Inter_600SemiBold', color: c.text }]} numberOfLines={1}>
          {item.title}
        </Text>
        <Text style={[metin.xs, { color: item.open_blockers ? c.danger : c.text3 }]} numberOfLines={1}>
          {clientSubtitle(item)}
        </Text>
      </View>
      <TacRozeti durum={tac} />
    </Pressable>
  );
}

const styles = StyleSheet.create({
  search: { flexDirection: 'row', alignItems: 'center', gap: 8, borderWidth: 1, borderRadius: 10, paddingHorizontal: 12 },
  add: { minHeight: DOKUNMA, borderRadius: 999, paddingHorizontal: 14, justifyContent: 'center' },
  chip: { minHeight: 32, borderRadius: 999, paddingHorizontal: 12, justifyContent: 'center' },
  row: { height: ROW_HEIGHT, flexDirection: 'row', alignItems: 'center', gap: 12, paddingHorizontal: 20, borderTopWidth: StyleSheet.hairlineWidth },
  av: { width: 36, height: 36, borderRadius: 9, alignItems: 'center', justifyContent: 'center' },
});

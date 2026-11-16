import { BosDurum, KristalYukleyici, TacRozeti } from '@hakan/mobile-core/hakan';
import { EkranBasligi, Hap, Satir } from '@hakan/mobile-core/ui/parcalar';
import { useTheme } from '@hakan/mobile-core/ui/theme';
import { useRouter } from 'expo-router';
import { useMemo } from 'react';
import { FlatList, RefreshControl, View } from 'react-native';
import { SafeAreaView } from 'react-native-safe-area-context';

import { Notice } from '@/components/ui';
import { errorMessage } from '@/core/api/envelope';
import { useClients } from '@/features/clients/api';

/**
 * Görev sekmesi: açık belge görevi olan mükellefler. Ofis genelinde görev
 * listeleme ucu yok; satır mükellefin görev listesini açar (`/tasks`).
 */
export default function OfficeTasksScreen() {
  const { colors: c } = useTheme();
  const router = useRouter();
  const q = useClients(undefined, 'tasks');
  const items = useMemo(() => q.data?.pages.flatMap((p) => p.items) ?? [], [q.data]);
  return (
    <SafeAreaView style={{ flex: 1, backgroundColor: c.bg }} edges={['top', 'left', 'right']}>
      <EkranBasligi baslik="Belge görevleri" />
      {q.error ? (
        <View style={{ padding: 16 }}>
          <Notice>{errorMessage(q.error)}</Notice>
        </View>
      ) : null}
      <FlatList
        data={items}
        keyExtractor={(x) => x.engagement_uuid}
        refreshControl={<RefreshControl refreshing={q.isRefetching && !q.isFetchingNextPage} onRefresh={() => void q.refetch()} tintColor={c.brandText} />}
        onEndReached={() => q.hasNextPage && !q.isFetchingNextPage && q.fetchNextPage()}
        ListEmptyComponent={
          q.isLoading ? (
            <View style={{ alignItems: 'center', paddingVertical: 40 }}>
              <KristalYukleyici etiket="Görevler yükleniyor" />
            </View>
          ) : (
            <BosDurum baslik="Açık belge görevi yok" aciklama="Mükellefe belge istediğinizde burada izlersiniz. Belge istemek için mükellefi açın." />
          )
        }
        renderItem={({ item, index }) => (
          <Satir
            testID="task-client-row"
            ilk={index === 0}
            sol={<TacRozeti durum="bekleniyor" />}
            baslik={item.title}
            alt={item.assigned_to_me ? 'Size atanmış' : undefined}
            sag={<Hap metin={`${item.open_tasks} açık`} ton="brand" />}
            accessibilityLabel={`${item.title}, ${item.open_tasks} açık görev`}
            onPress={() => router.push({ pathname: '/tasks', params: { engagement: item.engagement_uuid, title: item.title } })}
          />
        )}
      />
    </SafeAreaView>
  );
}

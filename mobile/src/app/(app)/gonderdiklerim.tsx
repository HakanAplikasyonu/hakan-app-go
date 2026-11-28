import { BosDurum, KristalYukleyici } from '@hakan/mobile-core/hakan';
import { router } from 'expo-router';
import { useMemo } from 'react';
import { Alert, FlatList, Pressable, RefreshControl, StyleSheet, View } from 'react-native';

import { Button, Chip, Text } from '@/components/ui';
import { useAuth } from '@/core/auth/auth-provider';
import { useColors } from '@/core/theme';
import { belgeyiAc } from '@/features/documents/ac';
import { gonderimSatirlari, type GonderimSatiri } from '@/features/receipts/gonderim';
import { useReceipts } from '@/features/receipts/provider';
import { useGonderimler } from '@/features/receipts/use-gonderimler';
import { taskTitle } from '@/features/tasks/tasks';
import { useTasks } from '@/features/tasks/use-tasks';

/**
 * Gönderdiklerim (#780 Senaryo B): telefondan gönderilen her fiş, Z raporu
 * ve belge isteği yüklemesi tek listede; durumu telefonda / gönderiliyor /
 * müşavirinize ulaştı / onaylandı / reddedildi (nedeniyle). Dokununca
 * fotoğraf ya da dosya görüntüleyicide açılır. Takılan gönderide neden,
 * "Tekrar dene" ve "Kuyruktan kaldır".
 */
export default function GonderdiklerimScreen() {
  const c = useColors();
  const { taxpayer } = useAuth();
  const tp = taxpayer?.taxpayer.uuid ?? '';
  const { items: queue, online, syncing, syncNow, retry, remove } = useReceipts();
  const server = useGonderimler(60_000);
  const { query: tasksQ } = useTasks();

  const rows = useMemo(() => {
    const titles: Record<string, string> = {};
    for (const t of tasksQ.data ?? []) titles[t.uuid] = taskTitle(t);
    return gonderimSatirlari(queue, server.items, tp, online, titles);
  }, [queue, server.items, tp, online, tasksQ.data]);

  const kaldir = (r: GonderimSatiri) => {
    if (!r.yerelId) return;
    const id = r.yerelId;
    Alert.alert('Kuyruktan kaldırılsın mı?', 'Fotoğraf telefondan silinir ve müşavirinize gönderilmez.', [
      { text: 'Vazgeç', style: 'cancel' },
      { text: 'Kaldır', style: 'destructive', onPress: () => remove(id) },
    ]);
  };

  if (server.isLoading && !rows.length) {
    return (
      <View style={{ flex: 1, alignItems: 'center', justifyContent: 'center', backgroundColor: c.background }}>
        <KristalYukleyici etiket="Gönderdikleriniz yükleniyor" />
      </View>
    );
  }

  return (
    <FlatList
      testID="gonderdiklerim-list"
      style={{ backgroundColor: c.background }}
      contentContainerStyle={{ padding: 16, gap: 10, maxWidth: 720, width: '100%', alignSelf: 'center' }}
      data={rows}
      keyExtractor={(r) => r.key}
      refreshControl={
        <RefreshControl
          refreshing={syncing || server.isRefetching}
          onRefresh={() => {
            syncNow();
            void server.refetch();
          }}
          tintColor={c.brand}
        />
      }
      ListHeaderComponent={
        <View style={{ gap: 4, paddingBottom: 4 }}>
          <Text variant="caption" tone="muted">
            {server.office ? `Müşavir ofisiniz: ${server.office}. ` : ''}
            {online ? 'Aşağı çekince yenilenir.' : 'Bağlantı yok; telefondakiler bağlanınca gönderilecek.'}
          </Text>
          {server.isError && online ? (
            <Text variant="caption" tone="danger">
              Ofisteki durumlar şu an alınamadı; telefondaki gönderiler gösteriliyor.
            </Text>
          ) : null}
        </View>
      }
      ListEmptyComponent={
        <BosDurum
          baslik="Henüz gönderi yok"
          aciklama="Fiş, Z raporu ya da istenen belgeyi çekip gönderdiğinizde burada durumunu görürsünüz."
          eylem={<Button title="Fiş çek" onPress={() => router.push('/capture')} />}
        />
      }
      renderItem={({ item: r }) => (
        <View testID="gonderim-row" style={[styles.row, { backgroundColor: c.surface, borderColor: r.takili ? c.warning : c.border }]}>
          <Pressable
            testID="gonderim-ac"
            accessibilityRole="button"
            accessibilityLabel={`${r.baslik}, ${r.etiket}. Açmak için dokunun`}
            disabled={!r.belge}
            onPress={() => r.belge && belgeyiAc(r.belge)}
            style={{ gap: 6 }}>
            <View style={{ flexDirection: 'row', alignItems: 'center', gap: 8 }}>
              <View style={{ flex: 1, gap: 2 }}>
                <Text style={{ fontWeight: '600' }} numberOfLines={1}>
                  {r.baslik}
                </Text>
                <Text variant="caption" tone="muted" numberOfLines={1}>
                  {r.alt}
                </Text>
              </View>
              <View testID={`gonderim-durum-${r.asama}`}>
                <Chip label={r.etiket} tone={r.ton} />
              </View>
            </View>
            {r.neden ? (
              <Text testID="gonderim-neden" variant="caption" tone={r.ton === 'danger' ? 'danger' : 'muted'}>
                {r.neden}
              </Text>
            ) : null}
          </Pressable>
          {r.takili && r.yerelId ? (
            <View style={{ flexDirection: 'row', gap: 8 }}>
              <View style={{ flex: 1 }}>
                <Button testID="gonderim-retry" title="Tekrar dene" variant="secondary" onPress={() => retry(r.yerelId!)} />
              </View>
              <View style={{ flex: 1 }}>
                <Button testID="gonderim-remove" title="Kuyruktan kaldır" variant="ghost" onPress={() => kaldir(r)} />
              </View>
            </View>
          ) : null}
        </View>
      )}
    />
  );
}

const styles = StyleSheet.create({
  row: { borderWidth: 1, borderRadius: 14, padding: 12, gap: 10 },
});

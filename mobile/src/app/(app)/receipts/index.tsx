import { Image } from 'expo-image';
import { router } from 'expo-router';
import { FlatList, RefreshControl, StyleSheet, View } from 'react-native';
import { SafeAreaView } from 'react-native-safe-area-context';

import { Button, Card, Chip, Row, Text } from '@/components/ui';
import { useAuth } from '@/core/auth/auth-provider';
import { useColors } from '@/core/theme';
import { kindLabels, statusLabel } from '@/features/receipts/fields';
import { historyStatusLabel, serverOnly } from '@/features/receipts/history';
import { useReceipts } from '@/features/receipts/provider';
import { receiptItems } from '@/features/receipts/queue';
import { QueueBanner } from '@/features/receipts/queue-banner';
import { shotAt, statusTone } from '@/features/receipts/status';
import { useReceiptHistory } from '@/features/receipts/use-history';

export default function ReceiptsScreen() {
  const { items: queue, syncing, syncNow } = useReceipts();
  const items = receiptItems(queue);
  const { history, loading, refresh } = useReceiptHistory();
  const { taxpayer } = useAuth();
  const earlier = serverOnly(history, items, taxpayer?.taxpayer.uuid ?? '');
  const c = useColors();
  return (
    <SafeAreaView style={{ flex: 1, backgroundColor: c.background }} edges={['left', 'right', 'bottom']}>
      <FlatList
        data={items}
        keyExtractor={(i) => i.id}
        contentContainerStyle={{ padding: 20, gap: 10, maxWidth: 640, width: '100%', alignSelf: 'center' }}
        refreshControl={
          <RefreshControl
            refreshing={syncing || loading}
            onRefresh={() => {
              syncNow();
              void refresh();
            }}
          />
        }
        ListHeaderComponent={
          <View style={{ gap: 10, marginBottom: 6 }}>
            <QueueBanner />
            <Button title="Fiş çek" onPress={() => router.push('/capture')} />
          </View>
        }
        ListFooterComponent={
          earlier.length ? (
            <View style={{ gap: 10, marginTop: 16 }}>
              <Text variant="heading">Daha önce gönderilenler</Text>
              {earlier.map((h) => (
                <Row
                  key={h.uuid}
                  title={h.vendor_name || (h.kind && kindLabels[h.kind as keyof typeof kindLabels]) || 'Fiş'}
                  subtitle={`${h.date ?? h.created_at.slice(0, 10)}${h.total_amount ? ` · ${h.total_amount} ₺` : ''}`}
                  accessibilityLabel={`${h.vendor_name ?? 'Fiş'}, ${historyStatusLabel(h.status)}`}
                  trailing={<Chip label={historyStatusLabel(h.status)} tone="neutral" />}
                />
              ))}
            </View>
          ) : undefined
        }
        ListEmptyComponent={
          <Card>
            <Text variant="heading">Henüz fiş yok</Text>
            <Text tone="muted">Masraf, yakıt fişi ya da Z raporu çekin; müşavirinize gönderelim.</Text>
          </Card>
        }
        renderItem={({ item }) => (
          <Row
            title={kindLabels[item.kind]}
            subtitle={`${shotAt(item.createdAt)}${item.result?.fields.total_amount?.value ? ` · ${item.result.fields.total_amount.value} ₺` : ''}`}
            accessibilityLabel={`${kindLabels[item.kind]}, ${statusLabel(item)}`}
            onPress={() => router.push({ pathname: '/receipts/[id]', params: { id: item.id } })}
            leading={
              <View style={[styles.thumb, { borderColor: c.border }]}>
                <Image source={{ uri: item.fileUri }} style={StyleSheet.absoluteFill} contentFit="cover" />
              </View>
            }
            trailing={<Chip label={statusLabel(item)} tone={statusTone(item)} />}
          />
        )}
      />
    </SafeAreaView>
  );
}

const styles = StyleSheet.create({
  thumb: { width: 44, height: 58, borderRadius: 8, overflow: 'hidden', borderWidth: StyleSheet.hairlineWidth },
});

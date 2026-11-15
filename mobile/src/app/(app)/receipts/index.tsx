import { Image } from 'expo-image';
import { router } from 'expo-router';
import { FlatList, RefreshControl, StyleSheet, View } from 'react-native';
import { SafeAreaView } from 'react-native-safe-area-context';

import { Button, Card, Chip, Row, Text } from '@/components/ui';
import { useColors } from '@/core/theme';
import { kindLabels, statusLabel } from '@/features/receipts/fields';
import { useReceipts } from '@/features/receipts/provider';
import { QueueBanner } from '@/features/receipts/queue-banner';
import { shotAt, statusTone } from '@/features/receipts/status';

export default function ReceiptsScreen() {
  const { items, syncing, syncNow } = useReceipts();
  const c = useColors();
  return (
    <SafeAreaView style={{ flex: 1, backgroundColor: c.background }} edges={['left', 'right', 'bottom']}>
      <FlatList
        data={items}
        keyExtractor={(i) => i.id}
        contentContainerStyle={{ padding: 20, gap: 10, maxWidth: 640, width: '100%', alignSelf: 'center' }}
        refreshControl={<RefreshControl refreshing={syncing} onRefresh={syncNow} />}
        ListHeaderComponent={
          <View style={{ gap: 10, marginBottom: 6 }}>
            <QueueBanner />
            <Button title="Fiş çek" onPress={() => router.push('/capture')} />
          </View>
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

import { router, useLocalSearchParams } from 'expo-router';
import { View } from 'react-native';

import { Button, Screen, Text } from '@/components/ui';
import { useReceipts } from '@/features/receipts/provider';
import { TeslimKarti } from '@/features/receipts/teslim-karti';

/**
 * Gönderim onayı (#780 Senaryo B): birden çok fotoğraf ya da Z raporu
 * "Gönder"le yollanınca açılır. Ulaşınca "Müşavirinize ulaştı ✓", ofisin adı
 * ve saati; bağlantı yoksa "Telefonda sırada" ve nedeni. Durum kuyruk
 * ilerledikçe kendiliğinden güncellenir.
 */
export default function GonderildiScreen() {
  const { ids: raw } = useLocalSearchParams<{ ids?: string }>();
  const ids = (raw ?? '').split(',').filter(Boolean);
  const { items, retry } = useReceipts();
  const failed = items.filter((i) => ids.includes(i.id) && (i.status === 'failed' || i.status === 'blocked'));

  return (
    <Screen style={{ maxWidth: 640, width: '100%', alignSelf: 'center' }}>
      <View testID="gonderildi" style={{ gap: 4 }}>
        <Text variant="title" accessibilityRole="header">
          {ids.length > 1 ? `${ids.length} fotoğraf gönderildi` : 'Fotoğraf gönderildi'}
        </Text>
        <Text variant="caption" tone="muted">
          Her gönderinin durumunu Gönderdiklerim ekranında görebilirsiniz.
        </Text>
      </View>
      <TeslimKarti ids={ids} />
      {failed.length ? (
        <Button testID="gonderildi-retry" title="Tekrar dene" variant="secondary" onPress={() => failed.forEach((i) => retry(i.id))} />
      ) : null}
      <Button testID="gonderildi-liste" title="Gönderdiklerim" onPress={() => router.replace('/gonderdiklerim')} />
      <Button testID="gonderildi-yeni" title="Yeni fiş çek" variant="secondary" onPress={() => router.replace('/capture')} />
      <Button testID="gonderildi-ana" title="Ana sayfa" variant="ghost" onPress={() => router.replace('/')} />
    </Screen>
  );
}

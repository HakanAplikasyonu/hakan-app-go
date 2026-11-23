import { Image } from 'expo-image';
import { ActivityIndicator, StyleSheet, View } from 'react-native';

import { Chip, Text } from '@/components/ui';
import { useOrganization } from '@/core/auth/auth-provider';
import { useColors } from '@/core/theme';
import { receiptImageSource } from '@/features/clients/api';

import { useDraftCapture } from './api';

/** Okunan alan: ad, değer; kontrol edilecekse "Kontrol et" rozeti. */
export function InfoRow({ label, value, check, testID }: { label: string; value: string | null | undefined; check?: boolean; testID?: string }) {
  return (
    <View style={styles.info} testID={testID}>
      <Text variant="caption" tone="muted" style={{ flex: 1 }}>
        {label}
      </Text>
      <View style={{ flex: 1.4, alignItems: 'flex-end', gap: 4 }}>
        <Text style={{ textAlign: 'right' }}>{value || '—'}</Text>
        {check ? <Chip tone="warning" label="Kontrol et" /> : null}
      </View>
    </View>
  );
}

/**
 * Masrafın / Z raporunun fiş fotoğrafı (yükleme kaydındaki görsel). Görsel
 * değilse (PDF) ya da bulunamazsa kısa not.
 */
export function ReceiptPhoto({ engagement, draft, period }: { engagement: string; draft: string; period: string | null }) {
  const org = useOrganization();
  const c = useColors();
  const capture = useDraftCapture(engagement, draft, period, true);
  if (capture.isLoading) return <ActivityIndicator color={c.brand} />;
  const r = capture.data;
  if (!r) return null;
  if (!r.media_type.startsWith('image/')) {
    return (
      <Text variant="caption" tone="muted">
        Belge PDF olarak yüklendi; görseli ofis panelinden açabilirsiniz.
      </Text>
    );
  }
  return (
    <Image
      testID="document-image"
      source={receiptImageSource(org, engagement, r.uuid)}
      style={{ width: '100%', aspectRatio: 3 / 4, borderRadius: 16, backgroundColor: c.border }}
      contentFit="contain"
      transition={150}
      accessibilityLabel="Fiş görseli"
    />
  );
}

const styles = StyleSheet.create({
  info: { flexDirection: 'row', alignItems: 'flex-start', gap: 12, minHeight: 28 },
});

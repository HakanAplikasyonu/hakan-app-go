import { Image } from 'expo-image';
import { Stack, useLocalSearchParams } from 'expo-router';
import { Platform, ScrollView, StyleSheet, useWindowDimensions, View } from 'react-native';
import { SafeAreaView } from 'react-native-safe-area-context';

import { Text } from '@/components/ui';
import { useOrganization } from '@/core/auth/auth-provider';
import { receiptImageSource } from '@/features/clients/api';

/**
 * Fiş görseli tam ekran (Oğuz'un canlı geri bildirimi): "Son fişler",
 * belge ayrıntısı ve onay ekranındaki küçük görsele dokununca açılır.
 * iOS'ta iki parmakla yakınlaştırılır (ScrollView yakınlaştırması; yeni
 * yerel bağımlılık yok); Android'de görsel ekrana sığdırılır.
 */
export default function ReceiptImageScreen() {
  const { engagement, receipt, title } = useLocalSearchParams<{ engagement: string; receipt: string; title?: string }>();
  const org = useOrganization();
  const { width, height } = useWindowDimensions();
  const zoom = Platform.OS === 'ios';
  return (
    <SafeAreaView style={styles.root} edges={['left', 'right', 'bottom']}>
      <Stack.Screen options={{ title: title || 'Fiş görseli' }} />
      <ScrollView
        style={styles.root}
        contentContainerStyle={styles.center}
        maximumZoomScale={zoom ? 5 : 1}
        minimumZoomScale={1}
        bouncesZoom
        centerContent
        showsHorizontalScrollIndicator={false}
        showsVerticalScrollIndicator={false}>
        <Image
          testID="receipt-viewer-image"
          source={receiptImageSource(org, engagement, receipt)}
          style={{ width, height: height * 0.8 }}
          contentFit="contain"
          transition={150}
          accessibilityLabel="Fiş görseli, tam ekran"
        />
      </ScrollView>
      {zoom ? (
        <View style={styles.hint} pointerEvents="none">
          <Text variant="caption" style={{ color: '#FFFFFF' }}>
            Yakınlaştırmak için iki parmağınızla açın.
          </Text>
        </View>
      ) : null}
    </SafeAreaView>
  );
}

const styles = StyleSheet.create({
  root: { flex: 1, backgroundColor: '#000000' },
  center: { flexGrow: 1, alignItems: 'center', justifyContent: 'center' },
  hint: { position: 'absolute', bottom: 24, left: 0, right: 0, alignItems: 'center' },
});

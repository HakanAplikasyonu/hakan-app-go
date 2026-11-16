// Kaynak: mobile/src/app/+not-found.tsx (#241'de kopyalandı).
import { router, Stack } from 'expo-router';
import { View } from 'react-native';

import { BrandMark } from '@/components/brand-mark';
import { Button, Screen, Text } from '@/components/ui';

/** Unknown route (#202): a dead link or an old notification. */
export default function NotFoundScreen() {
  return (
    <Screen style={{ justifyContent: 'center', alignItems: 'center', gap: 16 }}>
      <Stack.Screen options={{ headerShown: false }} />
      <BrandMark size={64} />
      <View style={{ gap: 6, alignItems: 'center' }}>
        <Text variant="title">Sayfa bulunamadı</Text>
        <Text tone="muted" style={{ textAlign: 'center' }}>
          Açmaya çalıştığınız bağlantı artık geçerli değil ya da bu uygulamada gösterilmiyor.
        </Text>
      </View>
      <Button title="Ana sayfaya dön" onPress={() => router.replace('/')} />
    </Screen>
  );
}

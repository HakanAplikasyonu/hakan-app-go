import { Stack } from 'expo-router';

import { useColors } from '@/core/theme';
import { ReceiptsProvider } from '@/features/receipts/provider';

export default function AppLayout() {
  const c = useColors();
  return (
    <ReceiptsProvider>
      <Stack
        screenOptions={{
          headerShown: false,
          contentStyle: { backgroundColor: c.background },
          headerStyle: { backgroundColor: c.background },
          headerTintColor: c.text,
          headerShadowVisible: false,
          headerBackButtonDisplayMode: 'minimal',
        }}>
        <Stack.Screen name="(tabs)" />
        <Stack.Screen name="capture" options={{ headerShown: true, title: 'Fiş çek', presentation: 'modal' }} />
        <Stack.Screen name="receipts/[id]" options={{ headerShown: true, title: 'Fiş' }} />
        <Stack.Screen name="task/[uuid]" options={{ headerShown: true, title: 'İstek' }} />
        <Stack.Screen name="notifications" options={{ headerShown: true, title: 'Bildirimler' }} />
      </Stack>
    </ReceiptsProvider>
  );
}

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
        <Stack.Screen name="index" />
        <Stack.Screen name="capture" options={{ headerShown: true, title: 'Fiş çek', presentation: 'modal' }} />
        <Stack.Screen name="receipts/index" options={{ headerShown: true, title: 'Fişlerim' }} />
        <Stack.Screen name="receipts/[id]" options={{ headerShown: true, title: 'Fiş' }} />
        <Stack.Screen name="tasks" options={{ headerShown: true, title: 'Görevler' }} />
        <Stack.Screen name="task/[uuid]" options={{ headerShown: true, title: 'Görev' }} />
        <Stack.Screen name="profile" options={{ headerShown: true, title: 'Profil ve hesap' }} />
      </Stack>
    </ReceiptsProvider>
  );
}

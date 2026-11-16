import { Stack } from 'expo-router';

import { useColors } from '@/core/theme';

export default function AppLayout() {
  const c = useColors();
  return (
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
      <Stack.Screen name="clients/[engagement]" options={{ headerShown: true, title: 'Mükellef' }} />
      <Stack.Screen name="tasks/index" options={{ headerShown: true, title: 'Belge görevleri' }} />
      <Stack.Screen name="tasks/new" options={{ presentation: 'modal' }} />
      <Stack.Screen name="tasks/[uuid]" options={{ headerShown: true, title: 'Görev' }} />
      <Stack.Screen name="review/[uuid]" options={{ headerShown: true, title: 'Öneri' }} />
      <Stack.Screen name="warnings/index" options={{ headerShown: true, title: 'Uyarılar' }} />
      <Stack.Screen name="warnings/[uuid]" options={{ headerShown: true, title: 'Uyarı' }} />
    </Stack>
  );
}

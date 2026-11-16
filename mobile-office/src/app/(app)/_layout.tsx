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
      <Stack.Screen name="index" />
      <Stack.Screen name="clients/index" options={{ headerShown: true, title: 'Mükellefler' }} />
      <Stack.Screen name="clients/[engagement]" options={{ headerShown: true, title: 'Mükellef' }} />
      <Stack.Screen name="warnings/index" options={{ headerShown: true, title: 'Uyarılar' }} />
      <Stack.Screen name="warnings/[uuid]" options={{ headerShown: true, title: 'Uyarı' }} />
    </Stack>
  );
}

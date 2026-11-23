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
      <Stack.Screen name="belgeler/[engagement]" options={{ headerShown: true, title: 'Belgeler' }} />
      <Stack.Screen name="donem/[engagement]" options={{ headerShown: true, title: 'Dönem' }} />
      <Stack.Screen name="belge/[uuid]" options={{ headerShown: true, title: 'Belge' }} />
      <Stack.Screen name="gorsel" options={{ headerShown: true, title: 'Fiş görseli' }} />
      <Stack.Screen name="tasks/index" options={{ headerShown: true, title: 'Belge görevleri' }} />
      <Stack.Screen name="tasks/new" options={{ presentation: 'modal' }} />
      <Stack.Screen name="tasks/[uuid]" options={{ headerShown: true, title: 'Görev' }} />
      <Stack.Screen name="review/[uuid]" options={{ headerShown: true, title: 'Öneri' }} />
      <Stack.Screen name="warnings/index" options={{ headerShown: true, title: 'Uyarılar' }} />
      <Stack.Screen name="warnings/[uuid]" options={{ headerShown: true, title: 'Uyarı' }} />
      <Stack.Screen name="destek" options={{ headerShown: true, title: 'Destek' }} />
      <Stack.Screen name="siradaki-is" options={{ headerShown: true, title: 'Sıradaki iş' }} />
      <Stack.Screen name="kdv-uyarilari" options={{ headerShown: true, title: 'KDV uyarıları' }} />
      <Stack.Screen name="gelen-kutusu" options={{ headerShown: true, title: 'Gelen kutusu' }} />
      <Stack.Screen name="mukellef-ekle" options={{ headerShown: true, title: 'Mükellef ekle' }} />
    </Stack>
  );
}

import { Stack } from 'expo-router';

import { useColors } from '@/core/theme';

export default function AppLayout() {
  const c = useColors();
  return <Stack screenOptions={{ headerShown: false, contentStyle: { backgroundColor: c.background } }} />;
}

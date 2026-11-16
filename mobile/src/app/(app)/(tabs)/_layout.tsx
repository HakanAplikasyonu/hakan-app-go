import { rayMi, rozet } from '@hakan/mobile-core/ui/layout';
import { Ikon } from '@hakan/mobile-core/ui/ikon';
import { SekmeCubugu, type Sekme } from '@hakan/mobile-core/ui/sekme-cubugu';
import { router, Tabs } from 'expo-router';
import { Pressable, useWindowDimensions } from 'react-native';

import { useSekmeRenkleri } from '@/features/home/renkler';
import { useTasks } from '@/features/tasks/use-tasks';

/**
 * Alt sekme çubuğu (MukellefMobil): Ana / İstekler / Geçmiş / Profil.
 * Tablette (≥ 768) solda 84 px ikon rayı ve en altta "Fiş çek" (MukellefTablet).
 */
export default function TabsLayout() {
  const { width } = useWindowDimensions();
  const ray = rayMi(width);
  const renk = useSekmeRenkleri();
  const { tasks } = useTasks();
  const sekmeler: Sekme[] = [
    { name: 'index', label: 'Ana', ikon: 'pano' },
    { name: 'tasks', label: 'İstekler', ikon: 'gorev', rozet: rozet(tasks.filter((t) => t.source === 'server').length) },
    { name: 'receipts/index', label: 'Geçmiş', ikon: 'gecmis', testID: 'tab-history' },
    { name: 'profile', label: 'Profil', ikon: 'profil' },
  ];
  return (
    <Tabs
      screenOptions={{ headerShown: false, tabBarPosition: ray ? 'left' : 'bottom', sceneStyle: { backgroundColor: renk.bg } }}
      tabBar={(p) => (
        <SekmeCubugu
          state={p.state}
          emitter={p.emitter}
          navigateToTab={p.navigateToTab}
          sekmeler={sekmeler}
          ray={ray}
          renk={renk}
          rayAlt={
            <Pressable
              testID="rail-capture"
              accessibilityRole="button"
              accessibilityLabel="Fiş çek"
              onPress={() => router.push('/capture')}
              style={({ pressed }) => ({ width: 60, height: 60, borderRadius: 18, backgroundColor: renk.brand, alignItems: 'center', justifyContent: 'center', opacity: pressed ? 0.85 : 1 })}>
              <Ikon ad="fisCek" boyut={22} renk={renk.onBrand} />
            </Pressable>
          }
        />
      )}>
      <Tabs.Screen name="index" />
      <Tabs.Screen name="tasks" />
      <Tabs.Screen name="receipts/index" />
      <Tabs.Screen name="profile" />
    </Tabs>
  );
}

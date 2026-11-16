import { unreadCount } from '@hakan/mobile-core/notifications/inbox';
import { rayMi, rozet } from '@hakan/mobile-core/ui/layout';
import { SekmeCubugu, type Sekme } from '@hakan/mobile-core/ui/sekme-cubugu';
import { Tabs } from 'expo-router';
import { useWindowDimensions } from 'react-native';

import { useDashboard } from '@/features/dashboard/api';
import { useInbox } from '@/features/notifications/api';
import { useSekmeRenkleri } from '@/features/notifications/renkler';

/**
 * Alt sekme çubuğu (MusavirMobil): Pano / Onay / Mükellef / Görev / Bildirim.
 * Tablette (≥ 768) solda ikon rayı (MusavirTablet).
 */
export default function TabsLayout() {
  const { width } = useWindowDimensions();
  const ray = rayMi(width);
  const renk = useSekmeRenkleri();
  const d = useDashboard().data;
  const inbox = useInbox();
  const sekmeler: Sekme[] = [
    { name: 'index', label: 'Pano', ikon: 'pano' },
    { name: 'review/index', label: 'Onay', ikon: 'kuyruk', rozet: rozet(d?.review?.pending), testID: 'tab-review' },
    { name: 'clients/index', label: 'Mükellef', ikon: 'mukellef', testID: 'tab-clients' },
    { name: 'gorevler', label: 'Görev', ikon: 'gorev', testID: 'tab-tasks' },
    { name: 'notifications', label: 'Bildirim', ikon: 'bildirim', rozet: rozet(unreadCount(inbox.items)), testID: 'tab-notifications' },
  ];
  return (
    <Tabs
      screenOptions={{ headerShown: false, tabBarPosition: ray ? 'left' : 'bottom', sceneStyle: { backgroundColor: renk.bg } }}
      tabBar={(p) => <SekmeCubugu state={p.state} emitter={p.emitter} navigateToTab={p.navigateToTab} sekmeler={sekmeler} ray={ray} renk={renk} />}>
      <Tabs.Screen name="index" />
      <Tabs.Screen name="review/index" />
      <Tabs.Screen name="clients/index" />
      <Tabs.Screen name="gorevler" />
      <Tabs.Screen name="notifications" />
    </Tabs>
  );
}

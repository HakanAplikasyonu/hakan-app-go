import { useTheme } from '@hakan/mobile-core/ui/theme';
import { useLocalSearchParams } from 'expo-router';
import { SafeAreaView } from 'react-native-safe-area-context';

import { IstekAyrintisi } from '@/features/tasks/istek-ayrintisi';

/** İstek (MukellefMobil · 3, #200): müşavirin istediği belge ve fotoğrafla yükleme. */
export default function TaskScreen() {
  const { uuid } = useLocalSearchParams<{ uuid: string }>();
  const { colors: c } = useTheme();
  return (
    <SafeAreaView style={{ flex: 1, backgroundColor: c.bg }} edges={['left', 'right', 'bottom']}>
      <IstekAyrintisi uuid={uuid ?? ''} />
    </SafeAreaView>
  );
}

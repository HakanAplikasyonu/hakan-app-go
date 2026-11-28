import { KristalYukleyici } from '@hakan/mobile-core/hakan';
import { StyleSheet, View } from 'react-native';

import { Text } from '@/components/ui';
import { useColors } from '@/core/theme';

import { teslimDurumu } from './gonderim';
import { useReceipts } from './provider';
import { useGonderimler } from './use-gonderimler';

/**
 * Gönderim onayı (#780 Senaryo B): fotoğraf gönderilince hemen görünür.
 * Ulaştıysa "Müşavirinize ulaştı ✓" ile ofisin adı ve saati; bağlantı yok ya
 * da gönderilemiyorsa "Telefonda sırada, bağlanınca gönderilecek" ve nedeni.
 */
export function TeslimKarti({ ids }: { ids: string[] }) {
  const { items, online } = useReceipts();
  const { office } = useGonderimler();
  const c = useColors();
  const mine = items.filter((i) => ids.includes(i.id));
  if (!mine.length) return null;
  const t = teslimDurumu(mine, online, office);
  const color = t.durum === 'ulasti' ? c.success : t.durum === 'gonderiliyor' ? c.brand : t.durum === 'sirada' ? c.warning : c.danger;
  return (
    <View
      testID={`teslim-${t.durum}`}
      accessibilityRole="summary"
      accessibilityLiveRegion="polite"
      style={[styles.card, { borderColor: color, backgroundColor: c.surface }]}>
      {t.durum === 'gonderiliyor' ? <KristalYukleyici boyut={22} gecikme={0} etiket={t.baslik} /> : null}
      <View style={{ flex: 1, gap: 2 }}>
        <Text testID="teslim-baslik" variant="heading" style={{ color }}>
          {t.baslik}
        </Text>
        <Text testID="teslim-aciklama" variant="caption" tone="muted">
          {t.aciklama}
        </Text>
      </View>
    </View>
  );
}

const styles = StyleSheet.create({
  card: { flexDirection: 'row', alignItems: 'center', gap: 12, borderWidth: 1.5, borderRadius: 14, padding: 14 },
});

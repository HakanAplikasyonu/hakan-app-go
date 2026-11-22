import { BosDurum, HataDurumu, KristalYukleyici } from '@hakan/mobile-core/hakan';
import { useTheme } from '@hakan/mobile-core/ui/theme';
import { useRouter } from 'expo-router';
import { FlatList, RefreshControl, View } from 'react-native';

import { Button, Chip, Row, Text } from '@/components/ui';
import { errorMessage } from '@/core/api/envelope';
import { money } from '@/features/clients/format';
import { useVatAlerts } from '@/features/vat-alerts/api';
import { dateLabel, eventLabel, eventTone, payableOf } from '@/features/vat-alerts/format';
import { periodLabel } from '@/features/warnings/format';

/**
 * KDV uyarıları (#799, mobilde #780): KDV nöbetçisinin ofis genelinde son
 * ürettiği uyarılar (eşik aşıldı, durum değişti, rapor hazır). Satır mükellef
 * ayrıntısını açar. Nöbetçi ofis panelindeki KDV takibi ayarından kapatılır.
 */
export default function VatAlertsScreen() {
  const q = useVatAlerts();
  const router = useRouter();
  const { colors: c } = useTheme();
  const items = q.data?.items ?? [];

  if (q.isLoading) {
    return (
      <View style={{ flex: 1, alignItems: 'center', justifyContent: 'center', backgroundColor: c.bg }}>
        <KristalYukleyici etiket="KDV uyarıları yükleniyor" />
      </View>
    );
  }

  return (
    <FlatList
      testID="vat-alerts-list"
      style={{ backgroundColor: c.bg }}
      contentContainerStyle={{ padding: 16, gap: 8 }}
      data={items}
      keyExtractor={(e) => e.uuid}
      refreshControl={<RefreshControl refreshing={q.isRefetching} onRefresh={() => void q.refetch()} tintColor={c.brandText} />}
      ListHeaderComponent={
        <View style={{ gap: 4, paddingBottom: 8 }}>
          <Text variant="heading" accessibilityRole="header">
            Son KDV uyarıları
          </Text>
          <Text tone="muted">Ödenecek KDV eşiği aşan ya da durumu değişen mükellefler. Mükellefe dokununca ayrıntısı açılır.</Text>
        </View>
      }
      ListEmptyComponent={
        q.error ? (
          <HataDurumu
            baslik="KDV uyarıları alınamadı"
            aciklama={errorMessage(q.error)}
            eylem={<Button testID="vat-alerts-retry" title="Tekrar dene" loading={q.isRefetching} onPress={() => void q.refetch()} />}
          />
        ) : (
          <BosDurum
            baslik="Henüz KDV uyarısı yok"
            aciklama="Nöbetçi her sabah çalışır; eşiği aşan ya da durumu değişen mükellef olunca burada görünür."
          />
        )
      }
      renderItem={({ item }) => {
        const payable = payableOf(item.payload);
        const sub = [periodLabel(item.period), payable ? `Ödenecek KDV ${money(payable)}` : null, dateLabel(item.created_at)]
          .filter(Boolean)
          .join(' · ');
        return (
          <View testID="vat-alert-row">
            <Row
              title={item.taxpayer_title}
              subtitle={sub}
              trailing={<Chip tone={eventTone(item.event)} label={eventLabel(item.event)} />}
              accessibilityLabel={`${item.taxpayer_title}. ${eventLabel(item.event)}. ${sub}.`}
              onPress={() =>
                router.push({ pathname: '/clients/[engagement]', params: { engagement: item.engagement_uuid, title: item.taxpayer_title } })
              }
            />
          </View>
        );
      }}
    />
  );
}

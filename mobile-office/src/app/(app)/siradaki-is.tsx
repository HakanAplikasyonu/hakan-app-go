import { BosDurum, HataDurumu, KristalYukleyici } from '@hakan/mobile-core/hakan';
import { useTheme } from '@hakan/mobile-core/ui/theme';
import { useRouter } from 'expo-router';
import { useMemo } from 'react';
import { FlatList, RefreshControl, View } from 'react-native';

import { Button, Chip, Row, Text } from '@/components/ui';
import { errorMessage } from '@/core/api/envelope';
import { useOfficeWorkflow } from '@/features/copilot/api';
import { idleLabel, nextStepOf, openFirst, progressLabel, stepDetail, stepLabels } from '@/features/copilot/format';
import { periodLabel } from '@/features/warnings/format';

/**
 * "Sıradaki iş" (#392, mobilde #454): ofisin mükelleflerinde dönem
 * kapanışının sıradaki adımı. İşi olanlar önce; satır mükellef ayrıntısını
 * (zincirin tamamı) açar. Salt okuma: adımlar ofis panelinde onaylanır.
 */
export default function NextWorkScreen() {
  const q = useOfficeWorkflow();
  const router = useRouter();
  const { colors: c } = useTheme();
  const rows = useMemo(() => openFirst(q.data?.items ?? []), [q.data]);
  const closed = rows.filter((r) => !r.next_step).length;

  if (q.isLoading) {
    return (
      <View style={{ flex: 1, alignItems: 'center', justifyContent: 'center', backgroundColor: c.bg }}>
        <KristalYukleyici etiket="Sıradaki iş yükleniyor" />
      </View>
    );
  }

  return (
    <FlatList
      testID="copilot-list"
      style={{ backgroundColor: c.bg }}
      contentContainerStyle={{ padding: 16, gap: 8 }}
      data={rows}
      keyExtractor={(r) => r.engagement_uuid}
      refreshControl={<RefreshControl refreshing={q.isRefetching} onRefresh={() => void q.refetch()} tintColor={c.brandText} />}
      ListHeaderComponent={
        q.data ? (
          <View style={{ gap: 4, paddingBottom: 8 }}>
            <Text variant="heading" accessibilityRole="header">
              Mükellefler · {periodLabel(q.data.period)}
            </Text>
            <Text tone="muted">Adımlar siz onaylamadan çalışmaz; onay ofis panelinde.</Text>
            {closed > 0 ? (
              <Text variant="caption" tone="muted">
                {closed} mükellefin dönemi kapandı
              </Text>
            ) : null}
          </View>
        ) : undefined
      }
      ListEmptyComponent={
        q.error ? (
          <HataDurumu
            baslik="Sıradaki iş alınamadı"
            aciklama={errorMessage(q.error)}
            eylem={<Button testID="copilot-retry" title="Tekrar dene" loading={q.isRefetching} onPress={() => void q.refetch()} />}
          />
        ) : (
          <BosDurum baslik="Mükellef bulunamadı" aciklama="Size atanmış ve aktif mükellefler burada listelenir." />
        )
      }
      renderItem={({ item }) => {
        const step = nextStepOf(item);
        return (
          <View testID="copilot-row">
            <Row
              title={item.taxpayer_title}
              subtitle={step ? `${stepLabels[step.key]}: ${stepDetail(step.key, step.status, step.count)}` : idleLabel(item)}
              trailing={<Chip tone={step ? 'brand' : 'success'} label={progressLabel(item)} />}
              accessibilityLabel={`${item.taxpayer_title}. ${step ? `Sıradaki iş: ${stepLabels[step.key]}` : idleLabel(item)}. ${progressLabel(item)}.`}
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

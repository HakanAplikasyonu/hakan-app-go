import { BosDurum, HataDurumu, KristalYukleyici } from '@hakan/mobile-core/hakan';
import { useTheme } from '@hakan/mobile-core/ui/theme';
import { useRouter } from 'expo-router';
import { useState } from 'react';
import { FlatList, Pressable, RefreshControl, View } from 'react-native';

import { Button, Chip, Notice, Row, Text } from '@/components/ui';
import { errorMessage } from '@/core/api/envelope';
import { useAuth } from '@/core/auth/auth-provider';
import { money } from '@/features/clients/format';
import { useApproveInboxItem, useInbox, type InboxItem } from '@/features/inbox/api';
import { badge, canApproveRow, inboxTypes, rowSubtitle, typeHints, typeLabel, type InboxType } from '@/features/inbox/format';
import { periodLabel } from '@/features/warnings/format';

/**
 * Gelen kutusu (#806, mobilde #780): bu dönem onay bekleyen fatura, banka
 * hareketi, masraf, Z raporu, yabancı fatura ve geç belge tek yerde. Türe
 * dokununca liste açılır; masraf ve Z satırları tek dokunuşla onaylanır
 * (`tenant.receipts.approve`), diğerleri mükellef ekranını açar.
 */
export default function InboxScreen() {
  const { can } = useAuth();
  const router = useRouter();
  const { colors: c } = useTheme();
  const [type, setType] = useState<InboxType | null>(null);
  const q = useInbox(type);
  const approve = useApproveInboxItem();
  const canApprove = can('tenant.receipts.approve');
  const counts = q.data?.counts;
  const items = q.data?.items ?? [];
  const period = q.data ? periodLabel(q.data.period) : null;

  const openTaxpayer = (item: InboxItem) =>
    router.push({ pathname: '/clients/[engagement]', params: { engagement: item.engagement_uuid, title: item.taxpayer_title } });

  if (q.isLoading && !q.data) {
    return (
      <View style={{ flex: 1, alignItems: 'center', justifyContent: 'center', backgroundColor: c.bg }}>
        <KristalYukleyici etiket="Gelen kutusu yükleniyor" />
      </View>
    );
  }

  return (
    <FlatList
      testID="inbox-list"
      style={{ backgroundColor: c.bg }}
      contentContainerStyle={{ padding: 16, gap: 8 }}
      data={items}
      keyExtractor={(i) => `${i.type}-${i.uuid}`}
      refreshControl={<RefreshControl refreshing={q.isRefetching} onRefresh={() => void q.refetch()} tintColor={c.brandText} />}
      ListHeaderComponent={
        <View style={{ gap: 8, paddingBottom: 8 }}>
          <Text variant="heading" accessibilityRole="header">
            Gelen kutusu{period ? ` · ${period}` : ''}
          </Text>
          <Text tone="muted">
            Bu dönem sizi bekleyen işler. Türe dokunun; masraf fişi ve Z raporunu tek dokunuşla onaylayın, diğerlerinde mükellef ekranı
            açılır.
          </Text>
          <View style={{ flexDirection: 'row', flexWrap: 'wrap', gap: 8 }} accessibilityLabel="Türe göre bekleyen işler">
            <TypeChip testID="inbox-type-all" label="Tümü" count={counts?.total} active={type === null} onPress={() => setType(null)} />
            {inboxTypes.map((t) => (
              <TypeChip
                key={t}
                testID={`inbox-type-${t}`}
                label={typeLabel(t)}
                count={counts?.[t]}
                active={type === t}
                onPress={() => setType(t)}
              />
            ))}
          </View>
          {type ? <Text tone="muted">{typeHints[type]}</Text> : null}
          {approve.error ? <Notice>{errorMessage(approve.error)}</Notice> : null}
        </View>
      }
      ListEmptyComponent={
        q.error ? (
          <HataDurumu
            baslik="Gelen kutusu alınamadı"
            aciklama={errorMessage(q.error)}
            eylem={<Button testID="inbox-retry" title="Tekrar dene" loading={q.isRefetching} onPress={() => void q.refetch()} />}
          />
        ) : (
          <BosDurum baslik="Bekleyen iş yok" aciklama="Bu dönem ve türde elinizi bekleyen kayıt kalmadı." />
        )
      }
      renderItem={({ item }) => {
        const sub = rowSubtitle(item, money);
        const showApprove = canApproveRow(item, canApprove);
        const busy = approve.isPending && approve.variables?.uuid === item.uuid;
        return (
          <View testID="inbox-row" style={{ gap: 6 }}>
            <Row
              title={item.taxpayer_title}
              subtitle={sub || undefined}
              trailing={<Chip tone={showApprove ? 'warning' : 'neutral'} label={typeLabel(item.type)} />}
              accessibilityLabel={`${item.taxpayer_title}. ${typeLabel(item.type)}. ${sub}.`}
              onPress={() => openTaxpayer(item)}
            />
            {showApprove ? (
              <Button
                testID="inbox-approve"
                title="Onayla"
                variant="secondary"
                loading={busy}
                disabled={approve.isPending}
                onPress={() => approve.mutate(item)}
              />
            ) : null}
          </View>
        );
      }}
    />
  );
}

function TypeChip({
  label,
  count,
  active,
  onPress,
  testID,
}: {
  label: string;
  count: number | undefined;
  active: boolean;
  onPress: () => void;
  testID: string;
}) {
  const { colors: c } = useTheme();
  const n = badge(count);
  return (
    <Pressable
      testID={testID}
      accessibilityRole="button"
      accessibilityState={{ selected: active }}
      accessibilityLabel={`${label}: ${count ?? 0} bekleyen`}
      onPress={onPress}
      style={({ pressed }) => ({
        flexDirection: 'row',
        alignItems: 'center',
        gap: 6,
        paddingHorizontal: 12,
        minHeight: 36,
        borderRadius: 18,
        borderWidth: 1,
        borderColor: active ? c.brand : c.line,
        backgroundColor: active ? c.brand : pressed ? c.surface2 : c.surface,
      })}>
      <Text style={{ color: active ? c.onBrand : c.text }}>{label}</Text>
      <Text style={{ color: active ? c.onBrand : c.text3 }}>{n ?? '0'}</Text>
    </Pressable>
  );
}

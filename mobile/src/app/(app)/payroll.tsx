import { BosDurum, HataDurumu, KristalYukleyici } from '@hakan/mobile-core/hakan';
import { Ikon } from '@hakan/mobile-core/ui/ikon';
import { metin, Satir } from '@hakan/mobile-core/ui/parcalar';
import { useTheme } from '@hakan/mobile-core/ui/theme';
import { useState } from 'react';
import { RefreshControl, SectionList, Text, View } from 'react-native';
import { SafeAreaView } from 'react-native-safe-area-context';

import { Button, Notice } from '@/components/ui';
import { errorMessage } from '@/core/api/envelope';
import { useAuth } from '@/core/auth/auth-provider';
import { groupByPeriod, openDocument, periodLabel, usePayrollDocuments, type PayrollDocument } from '@/features/payroll/api';

const TUR: Record<PayrollDocument['kind'], string> = {
  payroll: 'Bordro dökümü',
  payslip: 'Bordro pusulası',
  sgk_accrual: 'SGK tahakkuk fişi',
  sgk_receipt: 'SGK ödeme dekontu',
  other: 'Belge',
};

/**
 * Bordro belgeleri (#382): müşavirin paylaştığı bordro PDF'leri, döneme göre
 * gruplu; yalnızca okuma. Satıra dokununca PDF indirilip açılır.
 */
export default function PayrollScreen() {
  const { colors: c } = useTheme();
  const { taxpayer } = useAuth();
  const docs = usePayrollDocuments();
  const [error, setError] = useState<string | null>(null);
  const sections = groupByPeriod(docs.items).map((g) => ({ title: periodLabel(g.period), data: g.items }));

  async function open(d: PayrollDocument) {
    setError(null);
    try {
      await openDocument(taxpayer?.taxpayer.uuid ?? '', d);
    } catch {
      setError('Belge açılamadı. Bağlantınızı kontrol edip yeniden deneyin.');
    }
  }

  return (
    <SafeAreaView style={{ flex: 1, backgroundColor: c.bg }} edges={['left', 'right', 'bottom']}>
      {error || (docs.error && docs.items.length > 0) ? (
        <View style={{ padding: 16 }}>
          <Notice>{error ?? errorMessage(docs.error)}</Notice>
        </View>
      ) : null}
      <SectionList
        testID="payroll-list"
        sections={sections}
        keyExtractor={(d) => d.uuid}
        refreshControl={<RefreshControl refreshing={docs.isRefetching} onRefresh={() => void docs.refetch()} tintColor={c.brandText} />}
        renderSectionHeader={({ section }) => (
          <Text accessibilityRole="header" style={[metin.sm, { color: c.text2, fontFamily: 'Inter_600SemiBold', paddingHorizontal: 16, paddingTop: 16, paddingBottom: 6 }]}>
            {section.title}
          </Text>
        )}
        ListEmptyComponent={
          docs.isLoading ? (
            <View style={{ alignItems: 'center', paddingVertical: 40 }}>
              <KristalYukleyici etiket="Bordro belgeleri yükleniyor" />
            </View>
          ) : docs.error ? (
            // Liste hiç gelmediyse (çevrimdışı, sunucu hatası) "henüz yok" denmez (#454).
            <HataDurumu
              baslik="Bordro belgeleri alınamadı"
              aciklama={errorMessage(docs.error)}
              eylem={<Button testID="payroll-retry" title="Tekrar dene" loading={docs.isRefetching} onPress={() => void docs.refetch()} />}
            />
          ) : (
            <BosDurum
              baslik={docs.owner ? 'Henüz bordro belgesi yok' : 'Bordro belgeleri şirket sahibine açık'}
              aciklama={
                docs.owner
                  ? 'Müşavirinizin paylaştığı bordro PDF’leri burada görünür.'
                  : 'Bordro dökümleri bütün çalışanların ücretini taşıdığı için yalnızca şirket sahibi görür.'
              }
            />
          )
        }
        renderItem={({ item, index }) => (
          <Satir
            testID="payroll-row"
            ilk={index === 0}
            sol={<Ikon ad="klasor" renk={c.text2} />}
            baslik={item.title}
            alt={[TUR[item.kind], item.employee_name].filter(Boolean).join(' · ')}
            accessibilityLabel={`${item.title}, ${TUR[item.kind]}. Açmak için dokunun.`}
            onPress={() => void open(item)}
          />
        )}
      />
    </SafeAreaView>
  );
}

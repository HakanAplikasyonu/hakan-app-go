import { BosDurum, HataDurumu, KristalYukleyici } from '@hakan/mobile-core/hakan';
import { Ikon } from '@hakan/mobile-core/ui/ikon';
import { metin, Satir } from '@hakan/mobile-core/ui/parcalar';
import { useTheme } from '@hakan/mobile-core/ui/theme';
import { RefreshControl, SectionList, Text, View } from 'react-native';
import { SafeAreaView } from 'react-native-safe-area-context';

import { Button, Notice } from '@/components/ui';
import { errorMessage } from '@/core/api/envelope';
import { belgeyiAc } from '@/features/documents/ac';
import { arsivBelgesi, useImportantDocuments, useSharedDocuments } from '@/features/documents/api';
import { dosyaBoyutu } from '@/features/documents/belge';
import { belgeBolumleri, belgeEtiketi, gecerlilikMetni } from '@/features/documents/liste';

/**
 * Belgelerim (#780): müşavirin paylaştığı önemli belgeler (vergi levhası,
 * imza sirküleri…) ve "Mükellef de görsün" işaretli arşiv belgeleri.
 * Yalnızca şirket sahibi görür; satıra dokununca belge uygulama içi
 * görüntüleyicide açılır.
 */
export default function BelgelerimScreen() {
  const { colors: c } = useTheme();
  const important = useImportantDocuments();
  const shared = useSharedDocuments();
  const onemli = important.data?.enabled ? important.data.items : [];
  const sections = belgeBolumleri(onemli, shared.items);
  const error = shared.error ?? important.error;
  const loading = shared.isLoading || important.isLoading;
  const refreshing = shared.isRefetching || important.isRefetching;
  const refetch = () => {
    void shared.refetch();
    void important.refetch();
  };

  return (
    <SafeAreaView style={{ flex: 1, backgroundColor: c.bg }} edges={['left', 'right', 'bottom']}>
      {error && sections.length > 0 ? (
        <View style={{ padding: 16 }}>
          <Notice>{errorMessage(error)}</Notice>
        </View>
      ) : null}
      <SectionList
        testID="documents-list"
        sections={sections}
        keyExtractor={(d) => d.uuid}
        contentContainerStyle={{ paddingBottom: 24, maxWidth: 720, width: '100%', alignSelf: 'center' }}
        refreshControl={<RefreshControl refreshing={refreshing} onRefresh={refetch} tintColor={c.brandText} />}
        renderSectionHeader={({ section }) => (
          <Text accessibilityRole="header" style={[metin.sm, { color: c.text2, fontFamily: 'Inter_600SemiBold', paddingHorizontal: 16, paddingTop: 16, paddingBottom: 6 }]}>
            {section.title}
          </Text>
        )}
        ListEmptyComponent={
          loading ? (
            <View style={{ alignItems: 'center', paddingVertical: 40 }}>
              <KristalYukleyici etiket="Belgeler yükleniyor" />
            </View>
          ) : error ? (
            <HataDurumu
              baslik="Belgeler alınamadı"
              aciklama={errorMessage(error)}
              eylem={<Button testID="documents-retry" title="Tekrar dene" loading={refreshing} onPress={refetch} />}
            />
          ) : (
            <BosDurum
              baslik={shared.owner ? 'Henüz paylaşılan belge yok' : 'Belgeler şirket sahibine açık'}
              aciklama={
                shared.owner
                  ? 'Müşavirinizin sizinle paylaştığı vergi levhası, imza sirküleri, sözleşme gibi belgeler burada görünür.'
                  : 'Şirketin resmi belgelerini yalnızca şirket sahibi görür.'
              }
            />
          )
        }
        renderItem={({ item, index }) => {
          const sure = gecerlilikMetni(item);
          return (
            <Satir
              testID="document-row"
              ilk={index === 0}
              sol={<Ikon ad="klasor" renk={c.text2} />}
              baslik={item.title || item.file_name}
              alt={[belgeEtiketi(item), sure, dosyaBoyutu(item.size_bytes)].filter(Boolean).join(' · ')}
              altRenk={item.expiry_status === 'expired' ? c.danger : item.expiry_status === 'expiring' ? c.warn : undefined}
              accessibilityLabel={`${item.title || item.file_name}, ${belgeEtiketi(item)}${sure ? `, ${sure}` : ''}. Açmak için dokunun.`}
              onPress={() => belgeyiAc(arsivBelgesi(item))}
            />
          );
        }}
      />
    </SafeAreaView>
  );
}

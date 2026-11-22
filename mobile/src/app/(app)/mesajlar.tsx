import { BosDurum, HataDurumu, KristalYukleyici } from '@hakan/mobile-core/hakan';
import { Ikon } from '@hakan/mobile-core/ui/ikon';
import { Hap, metin, Satir } from '@hakan/mobile-core/ui/parcalar';
import { useTheme } from '@hakan/mobile-core/ui/theme';
import { useState } from 'react';
import { FlatList, RefreshControl, Text, View } from 'react-native';
import { SafeAreaView } from 'react-native-safe-area-context';

import { Button } from '@/components/ui';
import { errorMessage } from '@/core/api/envelope';
import { useMessages, type TaxpayerMessage } from '@/features/messages/api';
import { baslik, durum, kanalAdi, onizleme, zaman } from '@/features/messages/ozet';

/**
 * Mesajlarım (#780): mali müşavirin şirkete gönderdiği SMS ve e-postalar
 * (otomatik hatırlatmalar dahil), yeniden eskiye. Satıra dokununca mesajın
 * tamamı açılır. Kaçırılan bir hatırlatmayı (son ödeme günü, eksik belge)
 * buradan yeniden okumak içindir; salt okunur.
 */
export default function MesajlarScreen() {
  const { colors: c } = useTheme();
  const list = useMessages();
  const [open, setOpen] = useState<string | null>(null);

  function row({ item: m, index }: { item: TaxpayerMessage; index: number }) {
    const d = durum(m.status);
    const acik = open === m.uuid;
    return (
      <View>
        <Satir
          testID="message-row"
          ilk={index === 0}
          sol={<Ikon ad={m.channel === 'email' ? 'eposta' : 'talep'} renk={c.text2} />}
          baslik={baslik(m)}
          alt={`${kanalAdi(m.channel)} · ${zaman(m.sent_at ?? m.created_at)}${acik ? '' : ` · ${onizleme(m.body)}`}`}
          sag={<Hap metin={d.metin} ton={d.ton} />}
          onPress={() => setOpen(acik ? null : m.uuid)}
          accessibilityLabel={`${kanalAdi(m.channel)}, ${baslik(m)}, ${d.metin}. Tamamını ${acik ? 'gizlemek' : 'okumak'} için dokunun.`}
        />
        {acik ? (
          <View testID="message-body" style={{ paddingHorizontal: 16, paddingBottom: 14, gap: 6 }}>
            <Text selectable style={[metin.govde, { color: c.text }]}>
              {m.body.trim()}
            </Text>
            <Text style={[metin.sm, { color: c.text2 }]}>
              {[m.to ? `Gönderildiği yer: ${m.to}` : null, m.delivered_at ? `İletildi: ${zaman(m.delivered_at)}` : null]
                .filter(Boolean)
                .join(' · ')}
            </Text>
          </View>
        ) : null}
      </View>
    );
  }

  return (
    <SafeAreaView style={{ flex: 1, backgroundColor: c.bg }} edges={['left', 'right', 'bottom']}>
      <FlatList
        testID="messages-list"
        data={list.items}
        keyExtractor={(m) => m.uuid}
        renderItem={row}
        contentContainerStyle={{ maxWidth: 720, width: '100%', alignSelf: 'center', paddingBottom: 24 }}
        refreshControl={<RefreshControl refreshing={list.isRefetching} onRefresh={() => void list.refetch()} tintColor={c.brandText} />}
        ListHeaderComponent={
          <Text style={[metin.sm, { color: c.text2, padding: 16 }]}>
            Mali müşavirinizin size gönderdiği SMS ve e-postalar. Son ödeme günü ya da eksik belge hatırlatmasını kaçırdıysanız buradan yeniden
            okuyabilirsiniz. Tamamını görmek için mesaja dokunun.
          </Text>
        }
        ListFooterComponent={
          list.total > list.items.length ? (
            <Text style={[metin.sm, { color: c.text2, padding: 16, textAlign: 'center' }]}>Son {list.items.length} mesaj gösteriliyor.</Text>
          ) : undefined
        }
        ListEmptyComponent={
          list.isLoading ? (
            <View style={{ alignItems: 'center', paddingVertical: 40 }}>
              <KristalYukleyici etiket="Mesajlar yükleniyor" />
            </View>
          ) : list.error ? (
            <HataDurumu
              baslik="Mesajlar alınamadı"
              aciklama={errorMessage(list.error)}
              eylem={<Button testID="messages-retry" title="Tekrar dene" loading={list.isRefetching} onPress={() => void list.refetch()} />}
            />
          ) : (
            <BosDurum baslik="Henüz mesaj yok" aciklama="Mali müşavirinizin size gönderdiği SMS ve e-postalar burada görünür." />
          )
        }
      />
    </SafeAreaView>
  );
}

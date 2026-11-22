import { Ikon } from '@hakan/mobile-core/ui/ikon';
import { DOKUNMA } from '@hakan/mobile-core/ui/layout';
import { Hap, Kart, metin } from '@hakan/mobile-core/ui/parcalar';
import { useTheme } from '@hakan/mobile-core/ui/theme';
import { router } from 'expo-router';
import { Pressable, StyleSheet, Text, View } from 'react-native';

import { useForeignInvoices, useVatSales, type VatSales } from './api';
import { alindiMetni, ayAdi, kdvSonucu, sonGun, tl } from './ozet';

/**
 * Ana sayfada "Bu ayın KDV durumu" (#780): kesilen ve gelen faturalardan
 * anlık tahmin, ödenecek ya da devreden KDV ve son ödeme günü. Ofis paneli
 * kapattıysa ya da şirketin ofisi yoksa hiç görünmez.
 */
export function KdvDurumuKarti() {
  const { colors: c } = useTheme();
  const q = useVatSales();
  // Gizli ya da henüz yükleniyor: yer kaplamaz. Başka bir hata: kısa uyarı.
  if (q.hidden || (!q.data && !q.isError)) return null;
  return (
    <Kart testID="home-vat" style={styles.kart}>
      <Text accessibilityRole="header" style={[metin.govde, { fontFamily: 'Inter_700Bold', color: c.text }]}>
        Bu ayın KDV durumu
      </Text>
      {!q.data ? (
        <Text style={[metin.sm, { color: c.text3 }]}>KDV durumu şu an yüklenemedi. Biraz sonra yeniden deneyin.</Text>
      ) : (
        <VatBody data={q.data} />
      )}
    </Kart>
  );
}

function VatBody({ data }: { data: VatSales }) {
  const { colors: c } = useTheme();
  const sonuc = kdvSonucu(data.totals);
  const gun = sonGun(data.calendar);
  return (
    <View style={{ gap: 8 }}>
      <Text style={[metin.xs, { color: c.text3 }]}>
        {ayAdi(data.period)} · kesilen ve gelen faturalardan anlık tahmin. Kesin tutarı müşaviriniz beyanla belirler.
      </Text>
      <View style={styles.satir}>
        <Text style={[metin.sm, { color: c.text2, flex: 1 }]}>{sonuc.etiket}</Text>
        <Text testID="home-vat-amount" style={[styles.tutar, metin.num, { color: sonuc.odenecek ? c.text : c.brandText }]}>
          {sonuc.tutar}
        </Text>
      </View>
      <View style={styles.satir}>
        <Text style={[metin.xs, { color: c.text3, flex: 1 }]}>Satış KDV&apos;si</Text>
        <Text style={[metin.xs, metin.num, { color: c.text2 }]}>{tl(data.totals.output_vat)}</Text>
      </View>
      <View style={styles.satir}>
        <Text style={[metin.xs, { color: c.text3, flex: 1 }]}>Alış KDV&apos;si</Text>
        <Text style={[metin.xs, metin.num, { color: c.text2 }]}>{tl(data.totals.input_vat)}</Text>
      </View>
      {Number(data.totals.carried_in) > 0 ? (
        <View style={styles.satir}>
          <Text style={[metin.xs, { color: c.text3, flex: 1 }]}>Önceki aydan devreden</Text>
          <Text style={[metin.xs, metin.num, { color: c.text2 }]}>{tl(data.totals.carried_in)}</Text>
        </View>
      ) : null}
      <View style={styles.satir}>
        <Text style={[metin.xs, { color: gun.gecti ? c.danger : c.text2, flex: 1 }]}>{gun.metin}</Text>
        {gun.gecti ? <Hap metin="Gecikti" ton="danger" /> : gun.yakin ? <Hap metin="Yaklaştı" ton="warn" /> : null}
      </View>
    </View>
  );
}

/**
 * Ana sayfada "Yabancı hizmet faturası gönder" (#780 KDV2): bu ay kaç fatura
 * alındığını söyler, dokununca gönderme ekranı açılır. Ofis bölümü
 * kapattıysa kart görünmez.
 */
export function YabanciFaturaKarti() {
  const { colors: c } = useTheme();
  const q = useForeignInvoices();
  if (q.hidden || !q.data) return null;
  return (
    <Pressable
      testID="home-foreign"
      accessibilityRole="button"
      accessibilityLabel={`Yabancı hizmet faturası gönder. ${alindiMetni(q.count)}`}
      onPress={() => router.push('/yabanci-fatura')}
      style={({ pressed }) => ({ opacity: pressed ? 0.85 : 1 })}>
      <Kart style={[styles.kart, styles.yatay]}>
        <Ikon ad="yukle" renk={c.brandText} />
        <View style={{ flex: 1, gap: 2 }}>
          <Text style={[metin.sm, { fontFamily: 'Inter_600SemiBold', color: c.text }]}>Yabancı hizmet faturası gönder</Text>
          <Text style={[metin.xs, { color: c.text3 }]}>{alindiMetni(q.count)}</Text>
        </View>
        <Ikon ad="sag" boyut={16} renk={c.text3} />
      </Kart>
    </Pressable>
  );
}

const styles = StyleSheet.create({
  kart: { padding: 14, gap: 10 },
  yatay: { flexDirection: 'row', alignItems: 'center', gap: 12, minHeight: DOKUNMA },
  satir: { flexDirection: 'row', alignItems: 'center', gap: 8 },
  tutar: { fontFamily: 'InterTight_700Bold', fontSize: 20, lineHeight: 26 },
});

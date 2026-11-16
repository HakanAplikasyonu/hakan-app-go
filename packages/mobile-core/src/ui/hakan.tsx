/**
 * HAKAN'a özgü ortak bileşenler · Expo (arayüz v1, #324). Web
 * (frontend/src/components/hakan) ve Tauri (desktop/ui/hakan-ui.js) ile aynı
 * ad ve prop'lar. Ölçüler tuvalden: docs/tasarim/tuval/Kristal.dc.html.
 */
import { useEffect, useRef, useState, type ReactNode } from 'react';
import { AccessibilityInfo, Animated, Easing, Text as RNText, View, type ViewStyle } from 'react-native';
import Svg, { Polygon } from 'react-native-svg';

import { guvenKademesi, type GuvenKademesi, type GuvenKaynagi } from './theme-pref.ts';
import { useTheme } from './theme.ts';

export { guvenKademesi, type GuvenKademesi, type GuvenKaynagi };

export type TacDurumu = 'bekleniyor' | 'yuklendi' | 'onaylandi' | 'aktarildi' | 'sorun';
export type TacBoyutu = 16 | 30 | 72;

export const tacEtiketleri: Record<TacDurumu, string> = {
  bekleniyor: 'Bekleniyor',
  yuklendi: 'Yüklendi · öneri hazır',
  onaylandi: 'Müşavir onayladı',
  aktarildi: "Luca'ya aktarıldı",
  sorun: 'Sorun var',
};

const tacOlcu: Record<TacBoyutu, { w: number; gap: number; tepe: number }> = {
  16: { w: 5, gap: 2, tepe: 3 },
  30: { w: 9, gap: 3, tepe: 5 },
  72: { w: 22, gap: 6, tepe: 10 },
};

/** Sivri tepeli kristal (clip-path karşılığı). */
function Kristal({ w, h, tepe, fill, opacity }: { w: number; h: number; tepe: number; fill: string; opacity?: number }) {
  return (
    <Svg width={w} height={h} viewBox={`0 0 ${w} ${h}`}>
      <Polygon points={`0,${tepe} ${w / 2},0 ${w},${tepe} ${w},${h} 0,${h}`} fill={fill} opacity={opacity} />
    </Svg>
  );
}

/**
 * Taç rozeti: belge aşamasına göre dolan üç kristal. Durum yalnızca renkle
 * anlatılmaz: `etiket` açıksa metin yanında, değilse accessibilityLabel olur.
 */
export function TacRozeti({
  durum,
  boyut = 16,
  etiket = false,
  metin,
}: {
  durum: TacDurumu;
  boyut?: TacBoyutu;
  etiket?: boolean;
  metin?: string;
}) {
  const { colors: c } = useTheme();
  const label = metin ?? tacEtiketleri[durum];
  const { w, gap, tepe } = tacOlcu[boyut];
  const yan = Math.round((boyut * 11) / 16);
  const bos = boyut === 72 ? c.surface3 : c.lineStrong;
  const kucukSorun = durum === 'sorun' && boyut === 16;
  const sol = durum === 'bekleniyor' ? bos : kucukSorun ? c.danger : c.crystal.light;
  const orta = durum === 'aktarildi' ? c.crystal.mid : kucukSorun ? c.danger : bos;
  const sag = durum === 'onaylandi' || durum === 'aktarildi' ? c.crystal.dark : durum === 'sorun' ? c.danger : bos;
  const op = kucukSorun ? 0.85 : undefined;

  const tac = (
    <View
      accessible={!etiket}
      accessibilityRole={etiket ? undefined : 'image'}
      accessibilityLabel={etiket ? undefined : label}
      importantForAccessibility={etiket ? 'no-hide-descendants' : 'yes'}
      style={{ flexDirection: 'row', alignItems: 'flex-end', gap, height: boyut }}>
      <Kristal w={w} h={yan} tepe={tepe} fill={sol} opacity={op} />
      <Kristal w={w} h={boyut} tepe={tepe} fill={orta} opacity={op} />
      <Kristal w={w} h={yan} tepe={tepe} fill={sag} opacity={op} />
    </View>
  );
  if (!etiket) return tac;
  return (
    <View accessible accessibilityLabel={label} style={{ flexDirection: 'row', alignItems: 'center', gap: 8 }}>
      {tac}
      <RNText style={{ fontFamily: 'Inter_600SemiBold', fontSize: 13, color: durum === 'sorun' ? c.danger : c.text }}>
        {label}
      </RNText>
    </View>
  );
}

const kademeAd: Record<GuvenKademesi, string> = { dusuk: 'Düşük güven', orta: 'Orta güven', yuksek: 'Yüksek güven' };
const kaynakAd: Record<GuvenKaynagi, string> = {
  kural: 'kural eşleşmesi',
  gecmis: 'mizan hafızası',
  model: 'model önerisi',
  girdi: 'girdi',
};

/**
 * AI güven ölçeği: üç kademeli kristal. Yüzde yalnızca ekran okuyucuda ve
 * ayrıntı ekranında (mobilde "üzerine gelme" yok).
 */
export function GuvenOlcegi({ deger, kaynak, etiketli = false }: { deger: number; kaynak?: GuvenKaynagi; etiketli?: boolean }) {
  const { colors: c } = useTheme();
  const kademe = guvenKademesi(deger, kaynak);
  const renk = { dusuk: c.warn, orta: c.brandText, yuksek: c.ok }[kademe];
  const dolu = { dusuk: 1, orta: 2, yuksek: 3 }[kademe];
  const yuzde = `%${Math.round(Math.min(1, Math.max(0, deger)) * 100)}`;
  const label = [kademeAd[kademe], yuzde, kaynak ? kaynakAd[kaynak] : null].filter(Boolean).join(' · ');
  return (
    <View accessible accessibilityRole="image" accessibilityLabel={label} style={{ flexDirection: 'row', alignItems: 'center', gap: 8 }}>
      <View style={{ flexDirection: 'row', alignItems: 'flex-end', gap: 2 }}>
        {[6, 9, 12].map((h, i) => (
          <Kristal key={h} w={4} h={h} tepe={2} fill={i < dolu ? renk : c.lineStrong} />
        ))}
      </View>
      {etiketli ? (
        <RNText style={{ fontFamily: 'Inter_600SemiBold', fontSize: 12, color: c.text2 }}>{kademeAd[kademe]}</RNText>
      ) : null}
    </View>
  );
}

function useReduceMotion(): boolean {
  const [azalt, setAzalt] = useState(false);
  useEffect(() => {
    let canli = true;
    AccessibilityInfo.isReduceMotionEnabled()
      .then((v) => canli && setAzalt(v))
      .catch(() => {});
    const sub = AccessibilityInfo.addEventListener('reduceMotionChanged', setAzalt);
    return () => {
      canli = false;
      sub.remove();
    };
  }, []);
  return azalt;
}

/**
 * Kristal yükleyici: üç kristal sırayla yükselir (1.4 sn döngü, 180 ms
 * gecikme). 400 ms'den kısa işlemlerde görünmez. "Hareketi azalt" açıksa
 * yalnızca opaklık geçişi. Listeler bunun yerine iskelet kullanır.
 */
export function KristalYukleyici({ boyut = 40, etiket = 'Yükleniyor…', gecikme = 400 }: { boyut?: number; etiket?: string; gecikme?: number }) {
  const { colors: c } = useTheme();
  const azalt = useReduceMotion();
  const [gorunur, setGorunur] = useState(gecikme <= 0);
  const degerler = useRef([0, 1, 2].map(() => new Animated.Value(0))).current;

  useEffect(() => {
    if (gecikme <= 0) return;
    const id = setTimeout(() => setGorunur(true), gecikme);
    return () => clearTimeout(id);
  }, [gecikme]);

  useEffect(() => {
    const dongu = Animated.loop(
      Animated.stagger(
        180,
        degerler.map((v) =>
          Animated.sequence([
            Animated.timing(v, { toValue: 1, duration: 630, easing: Easing.bezier(0.2, 0.7, 0.2, 1), useNativeDriver: true }),
            Animated.timing(v, { toValue: 0, duration: 770, easing: Easing.bezier(0.2, 0.7, 0.2, 1), useNativeDriver: true }),
          ]),
        ),
      ),
    );
    dongu.start();
    return () => dongu.stop();
  }, [degerler]);

  const w = Math.max(4, Math.round((boyut * 12) / 40));
  const yan = Math.round((boyut * 28) / 40);
  const tepe = Math.max(2, Math.round((boyut * 6) / 40));
  const renkler = [c.crystal.light, c.crystal.mid, c.crystal.dark];
  return (
    <View
      accessible
      accessibilityRole="progressbar"
      accessibilityLabel={etiket}
      style={{ flexDirection: 'row', alignItems: 'flex-end', gap: Math.max(2, Math.round(boyut / 10)), height: boyut, opacity: gorunur ? 1 : 0 }}>
      {degerler.map((v, i) => {
        const h = i === 1 ? boyut : yan;
        const opacity = v.interpolate({ inputRange: [0, 1], outputRange: [0.35, 1] });
        // scaleY tabandan: yüksekliğin yarısı kadar kaydırarak.
        const scaleY = v.interpolate({ inputRange: [0, 1], outputRange: [0.35, 1] });
        const translateY = v.interpolate({ inputRange: [0, 1], outputRange: [(h * 0.65) / 2, 0] });
        const style = azalt ? { opacity } : { opacity, transform: [{ translateY }, { scaleY }] };
        return (
          <Animated.View key={i} style={style}>
            <Kristal w={w} h={h} tepe={tepe} fill={renkler[i]} />
          </Animated.View>
        );
      })}
    </View>
  );
}

/** Kesik çizgili kristaller (boş durum işareti). */
export function BosKristal() {
  const { colors: c } = useTheme();
  const k = (h: number) => (
    <Svg width={34} height={h} viewBox={`0 0 34 ${h}`}>
      <Polygon points={`1,14 17,1 33,14 33,${h} 1,${h}`} fill={c.surface2} stroke={c.lineStrong} strokeWidth={2} strokeDasharray="4 3" />
    </Svg>
  );
  return (
    <View importantForAccessibility="no-hide-descendants" style={{ flexDirection: 'row', alignItems: 'flex-end', gap: 8, height: 80 }}>
      {k(56)}
      {k(80)}
      {k(56)}
    </View>
  );
}

/** Boş durum: kesik çizgili kristaller, başlık, açıklama ve tek eylem. */
export function BosDurum({ baslik, aciklama, eylem, style }: { baslik: string; aciklama?: string; eylem?: ReactNode; style?: ViewStyle }) {
  const { colors: c } = useTheme();
  return (
    <View style={[{ alignItems: 'center', gap: 14, paddingVertical: 32, paddingHorizontal: 20 }, style]}>
      <BosKristal />
      <RNText accessibilityRole="header" style={{ fontFamily: 'InterTight_700Bold', fontSize: 20, lineHeight: 26, color: c.text, textAlign: 'center' }}>
        {baslik}
      </RNText>
      {aciklama ? (
        <RNText style={{ fontFamily: 'Inter_400Regular', fontSize: 15, lineHeight: 22, color: c.text2, textAlign: 'center', maxWidth: 380 }}>
          {aciklama}
        </RNText>
      ) : null}
      {eylem}
    </View>
  );
}

const M3 = [
  '0,86 28,59 28,198 0,198',
  '28,59 56,86 56,198 28,198',
  '63,36 91,0 91,198 63,198',
  '91,0 119,36 119,198 91,198',
  '125,86 153,59 153,198 125,198',
  '153,59 181,86 181,198 153,198',
];

/**
 * M3 logosu. `isaret`: kristaller; `yazi`: "hakan"; `tam`: ikisi. Kristal
 * yüzleri temadan gelir (N3'te kendiliğinden değişir). `zemin="marka"`:
 * mavi karo üstünde, orta kristal beyaz.
 */
export function MarkaLogosu({
  varyant = 'isaret',
  zemin = 'normal',
  boyut = 22,
  etiket = 'Hakan',
}: {
  varyant?: 'isaret' | 'yazi' | 'tam';
  zemin?: 'normal' | 'marka';
  boyut?: number;
  etiket?: string;
}) {
  const { colors: c } = useTheme();
  const mid = zemin === 'marka' ? '#FFFFFF' : c.crystal.mid;
  const fills = [c.crystal.light, mid, mid, c.crystal.dark, c.crystal.dark, c.crystal.light];
  const isaret = (
    <Svg width={Math.round((boyut * 181) / 198)} height={boyut} viewBox="0 0 181 198">
      {M3.map((p, i) => (
        <Polygon key={p} points={p} fill={fills[i]} />
      ))}
    </Svg>
  );
  const yazi = (
    <RNText style={{ fontFamily: 'InterTight_800ExtraBold', fontSize: Math.round(boyut * 0.86), letterSpacing: -0.05 * boyut * 0.86, color: zemin === 'marka' ? '#FFFFFF' : c.text }}>
      hakan
    </RNText>
  );
  return (
    <View accessible accessibilityRole="image" accessibilityLabel={etiket} style={{ flexDirection: 'row', alignItems: 'center', gap: 10 }}>
      {varyant !== 'yazi' ? isaret : null}
      {varyant !== 'isaret' ? yazi : null}
    </View>
  );
}

/**
 * Arayüz v1 ekran parçaları (iki uygulama ortak): kesik köşe, segment, hap,
 * ilerleme çubuğu, ekran başlığı, satır. Ölçüler tuvalden
 * (docs/tasarim/tuval/hakan.css, MusavirMobil, MukellefMobil). Renkler
 * `useTheme().colors`; dokunma hedefleri en az 44 px.
 */
import type { ReactNode } from 'react';
import { Pressable, StyleSheet, Text, View, type StyleProp, type ViewStyle } from 'react-native';

import { DOKUNMA } from './layout.ts';
import { setThemePreference, useTheme, useThemePreference } from './theme.ts';
import { tokens } from './tokens.ts';

const f = tokens.font;

/**
 * "Kristal kesik köşe": sağ üstte 14 px pah. RN'de clip-path olmadığı için
 * köşeye zemin renginde bir üçgen çizilir. Yalnızca vurgulu kartlarda
 * (pano özeti, onay kartı, çekim kartı).
 */
export function KesikKose({ zemin, boyut = 14 }: { zemin: string; boyut?: number }) {
  return (
    <View
      pointerEvents="none"
      style={{
        position: 'absolute',
        top: 0,
        right: 0,
        width: 0,
        height: 0,
        borderTopWidth: boyut,
        borderLeftWidth: boyut,
        borderTopColor: zemin,
        borderLeftColor: 'transparent',
      }}
    />
  );
}

/** Ekran başlığı: Inter Tight 26, solda başlık, sağda isteğe bağlı öğe. */
export function EkranBasligi({ baslik, ust, sag }: { baslik: string; ust?: string; sag?: ReactNode }) {
  const { colors: c } = useTheme();
  return (
    <View style={styles.head}>
      <View style={{ flex: 1 }}>
        {ust ? <Text style={[styles.ust, { color: c.text3 }]}>{ust}</Text> : null}
        <Text accessibilityRole="header" style={[styles.baslik, { color: c.text }]} numberOfLines={2}>
          {baslik}
        </Text>
      </View>
      {sag}
    </View>
  );
}

export type HapTonu = 'notr' | 'brand' | 'ok' | 'warn' | 'danger';

/** Hap (badge pill): 22 px, yuvarlak. */
export function Hap({ metin, ton = 'notr', style }: { metin: string; ton?: HapTonu; style?: StyleProp<ViewStyle> }) {
  const { colors: c } = useTheme();
  const renk = {
    notr: [c.surface3, c.text2],
    brand: [c.brandTint, c.brandText],
    ok: [c.okTint, c.ok],
    warn: [c.warnTint, c.warn],
    danger: [c.dangerTint, c.danger],
  }[ton];
  return (
    <View style={[styles.hap, { backgroundColor: renk[0] }, style]}>
      <Text style={[styles.hapText, { color: renk[1] }]} numberOfLines={1}>
        {metin}
      </Text>
    </View>
  );
}

/** Segment (Tabs / Segmented): `surface-3` yuvası, seçili segment yüzey rengi ve gölgeli. */
export function Segment<K extends string>({
  secenekler,
  secili,
  onSec,
  testIDOnek,
  style,
}: {
  secenekler: readonly { key: K; label: string }[];
  secili: K;
  onSec: (k: K) => void;
  testIDOnek?: string;
  style?: StyleProp<ViewStyle>;
}) {
  const { colors: c } = useTheme();
  return (
    <View accessibilityRole="tablist" style={[styles.segment, { backgroundColor: c.surface3 }, style]}>
      {secenekler.map((s) => {
        const on = s.key === secili;
        return (
          <Pressable
            key={s.key}
            testID={testIDOnek ? `${testIDOnek}-${s.key}` : undefined}
            accessibilityRole="tab"
            accessibilityState={{ selected: on }}
            onPress={() => onSec(s.key)}
            style={[styles.seg, on ? [styles.segOn, { backgroundColor: c.surface }] : null]}>
            <Text style={[styles.segText, { color: on ? c.text : c.text2 }]} numberOfLines={1}>
              {s.label}
            </Text>
          </Pressable>
        );
      })}
    </View>
  );
}

/** İlerleme çubuğu (0..1). */
export function IlerlemeCubugu({ oran, etiket }: { oran: number; etiket: string }) {
  const { colors: c } = useTheme();
  const v = Math.max(0, Math.min(1, oran));
  return (
    <View
      accessible
      accessibilityRole="progressbar"
      accessibilityLabel={etiket}
      accessibilityValue={{ min: 0, max: 100, now: Math.round(v * 100) }}
      style={[styles.bar, { backgroundColor: c.surface3 }]}>
      <View style={{ width: `${Math.max(2, v * 100)}%`, height: '100%', borderRadius: 3, backgroundColor: c.brand }} />
    </View>
  );
}

/** Kart: yüzey rengi, ince çizgi, köşe 14. */
export function Kart({ children, style, testID }: { children: ReactNode; style?: StyleProp<ViewStyle>; testID?: string }) {
  const { colors: c } = useTheme();
  return (
    <View testID={testID} style={[styles.kart, { backgroundColor: c.surface, borderColor: c.line }, style]}>
      {children}
    </View>
  );
}

/** Liste satırı: solda öğe (taç), başlık + alt satır, sağda öğe; dokunulursa en az 44 px. */
export function Satir({
  baslik,
  alt,
  altRenk,
  sol,
  sag,
  onPress,
  vurgulu,
  ilk,
  testID,
  accessibilityLabel,
}: {
  baslik: string;
  alt?: string;
  altRenk?: string;
  sol?: ReactNode;
  sag?: ReactNode;
  onPress?: () => void;
  vurgulu?: boolean;
  ilk?: boolean;
  testID?: string;
  accessibilityLabel?: string;
}) {
  const { colors: c } = useTheme();
  return (
    <Pressable
      testID={testID}
      disabled={!onPress}
      onPress={onPress}
      accessibilityRole={onPress ? 'button' : undefined}
      accessibilityLabel={accessibilityLabel}
      style={({ pressed }) => [
        styles.satir,
        { borderTopColor: c.line, borderTopWidth: ilk ? 0 : StyleSheet.hairlineWidth },
        vurgulu ? { backgroundColor: c.brandTint } : null,
        pressed ? { backgroundColor: c.surface2 } : null,
      ]}>
      {sol}
      <View style={{ flex: 1, gap: 2 }}>
        <Text style={[metin.sm, { fontFamily: f.sansSemi, color: c.text }]} numberOfLines={2}>
          {baslik}
        </Text>
        {alt ? (
          <Text style={[metin.xs, { color: altRenk ?? c.text3 }]} numberOfLines={2}>
            {alt}
          </Text>
        ) : null}
      </View>
      {sag}
    </Pressable>
  );
}

/** Ayarlar → Görünüm: palet (M3 / N3) ve kip (sistemi izle / açık / koyu). Seçim cihazda saklanır. */
export function GorunumAyari() {
  const { colors: c } = useTheme();
  const pref = useThemePreference();
  return (
    <View style={{ gap: 10 }}>
      <Text accessibilityRole="header" style={[metin.sm, { fontFamily: f.sansSemi, color: c.text }]}>
        Görünüm
      </Text>
      <Segment
        testIDOnek="tema-palet"
        secenekler={[
          { key: 'm3', label: 'M3 · mavi' },
          { key: 'n3', label: 'N3 · yeşil' },
        ]}
        secili={pref.palette}
        onSec={(palette) => setThemePreference({ palette })}
      />
      <Segment
        testIDOnek="tema-kip"
        secenekler={[
          { key: 'system', label: 'Sistemi izle' },
          { key: 'light', label: 'Açık' },
          { key: 'dark', label: 'Koyu' },
        ]}
        secili={pref.mode}
        onSec={(mode) => setThemePreference({ mode })}
      />
    </View>
  );
}

/** Metin ölçüleri (tuval `t-sm`, `t-xs`, `t-cap`, `kpi`). */
export const metin = StyleSheet.create({
  sm: { fontFamily: f.sans, fontSize: 14, lineHeight: 20 },
  xs: { fontFamily: f.sans, fontSize: 12.5, lineHeight: 17 },
  cap: { fontFamily: f.mono, fontSize: 11, lineHeight: 14, letterSpacing: 0.6, textTransform: 'uppercase' },
  kpi: { fontFamily: f.display, fontVariant: ['tabular-nums'], letterSpacing: -0.5 },
  num: { fontVariant: ['tabular-nums'] },
  mono: { fontFamily: f.mono },
  govde: { fontFamily: f.sans, fontSize: tokens.type.body.size, lineHeight: tokens.type.body.line },
  baslik2: { fontFamily: f.display, fontSize: 22, lineHeight: 28 },
});

const styles = StyleSheet.create({
  head: { flexDirection: 'row', alignItems: 'center', gap: 10, paddingHorizontal: 20, paddingTop: 12, paddingBottom: 14 },
  ust: { fontFamily: f.sans, fontSize: 14, lineHeight: 20 },
  baslik: { fontFamily: f.display, fontSize: 26, lineHeight: 32, letterSpacing: -0.4 },
  hap: { minHeight: 22, borderRadius: 999, paddingHorizontal: 9, alignItems: 'center', justifyContent: 'center', alignSelf: 'flex-start' },
  hapText: { fontFamily: f.sansSemi, fontSize: 12, fontVariant: ['tabular-nums'] },
  segment: { flexDirection: 'row', gap: 2, padding: 3, borderRadius: 10 },
  seg: { flex: 1, minHeight: DOKUNMA, borderRadius: 8, alignItems: 'center', justifyContent: 'center', paddingHorizontal: 8 },
  segOn: { shadowColor: '#0A0A0A', shadowOpacity: 0.08, shadowRadius: 6, shadowOffset: { width: 0, height: 1 }, elevation: 1 },
  segText: { fontFamily: f.sansSemi, fontSize: 13 },
  bar: { height: 6, borderRadius: 3, overflow: 'hidden' },
  kart: { borderWidth: StyleSheet.hairlineWidth, borderRadius: tokens.radius.lg, overflow: 'hidden' },
  satir: { flexDirection: 'row', alignItems: 'center', gap: 12, paddingHorizontal: 14, paddingVertical: 12, minHeight: DOKUNMA + 8 },
});

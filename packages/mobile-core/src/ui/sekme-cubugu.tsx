import type { ReactNode } from 'react';
import { Pressable, StyleSheet, Text, View } from 'react-native';
import { useSafeAreaInsets } from 'react-native-safe-area-context';

import { Ikon, type IkonAdi } from './ikon.tsx';
import { DOKUNMA } from './layout.ts';

export type Sekme = { name: string; label: string; ikon: IkonAdi; rozet?: string | null; testID?: string };

export type SekmeRenkleri = { surface: string; line: string; text3: string; brandText: string; brandTint: string; danger: string; onDanger: string };

/** react-navigation alt sekme çubuğunun kullandığımız kısmı (expo-router `Tabs` `tabBar` prop'u). */
type TabBarState = { index: number; routes: { key: string; name: string }[] };
type TabBarEmitter = { emit: (e: { type: 'tabPress'; target: string; canPreventDefault: true }) => { defaultPrevented: boolean } };

/**
 * Alt sekme çubuğu (telefon) ve ikon rayı (tablet). Tasarım: MukellefMobil,
 * MusavirMobil (`.tabbar`), MukellefTablet (`.rail`, 84 px). Her sekme en az
 * 44 px; seçili sekme `brand-text` ve ekran okuyucuda "seçili".
 */
export function SekmeCubugu({
  state,
  emitter,
  navigateToTab,
  sekmeler,
  ray,
  renk,
  rayUst,
  rayAlt,
}: {
  state: TabBarState;
  emitter: TabBarEmitter;
  navigateToTab: (routeKey: string) => void;
  sekmeler: readonly Sekme[];
  ray: boolean;
  renk: SekmeRenkleri;
  /** Rayın tepesi (logo). */
  rayUst?: ReactNode;
  /** Rayın dibi (ör. mükellefte "Fiş çek" düğmesi). */
  rayAlt?: ReactNode;
}) {
  const insets = useSafeAreaInsets();
  const routes = state.routes.filter((r) => sekmeler.some((s) => s.name === r.name));
  const focused = state.routes[state.index]?.key;

  const items = routes.map((r) => {
    const s = sekmeler.find((x) => x.name === r.name)!;
    const on = r.key === focused;
    const color = on ? renk.brandText : renk.text3;
    return (
      <Pressable
        key={r.key}
        testID={s.testID ?? `tab-${s.name}`}
        accessibilityRole="tab"
        accessibilityState={{ selected: on }}
        accessibilityLabel={s.rozet ? `${s.label}, ${s.rozet} yeni` : s.label}
        onPress={() => {
          const e = emitter.emit({ type: 'tabPress', target: r.key, canPreventDefault: true });
          if (!on && !e.defaultPrevented) navigateToTab(r.key);
        }}
        style={({ pressed }) => [ray ? styles.railItem : styles.tab, ray && on ? { backgroundColor: renk.brandTint } : null, { opacity: pressed ? 0.7 : 1 }]}>
        <View>
          <Ikon ad={s.ikon} boyut={22} renk={color} />
          {s.rozet ? (
            <View style={[styles.badge, { backgroundColor: renk.danger }]}>
              <Text style={[styles.badgeText, { color: renk.onDanger }]} allowFontScaling={false}>
                {s.rozet}
              </Text>
            </View>
          ) : null}
        </View>
        <Text style={[styles.label, { color }]} numberOfLines={1} maxFontSizeMultiplier={1.3}>
          {s.label}
        </Text>
      </Pressable>
    );
  });

  if (ray) {
    return (
      <View
        accessibilityRole="tablist"
        style={[styles.rail, { backgroundColor: renk.surface, borderRightColor: renk.line, paddingTop: insets.top + 20, paddingBottom: insets.bottom + 20, paddingLeft: insets.left }]}>
        {rayUst}
        {items}
        <View style={{ flex: 1 }} />
        {rayAlt}
      </View>
    );
  }
  return (
    <View
      accessibilityRole="tablist"
      style={[styles.bar, { backgroundColor: renk.surface, borderTopColor: renk.line, paddingBottom: Math.max(insets.bottom, 10), paddingLeft: insets.left, paddingRight: insets.right }]}>
      {items}
    </View>
  );
}

const styles = StyleSheet.create({
  bar: { flexDirection: 'row', justifyContent: 'space-around', alignItems: 'flex-start', paddingTop: 6, paddingHorizontal: 8, borderTopWidth: StyleSheet.hairlineWidth },
  tab: { flex: 1, minWidth: 56, minHeight: DOKUNMA + 6, alignItems: 'center', justifyContent: 'center', gap: 4 },
  rail: { width: 84, alignItems: 'center', gap: 8, borderRightWidth: StyleSheet.hairlineWidth },
  railItem: { width: 68, minHeight: 56, borderRadius: 12, alignItems: 'center', justifyContent: 'center', gap: 4, paddingVertical: 8 },
  label: { fontSize: 10.5, fontWeight: '600' },
  badge: { position: 'absolute', top: -4, right: -10, minWidth: 16, height: 16, borderRadius: 8, paddingHorizontal: 4, alignItems: 'center', justifyContent: 'center' },
  badgeText: { fontSize: 10, fontWeight: '700' },
});

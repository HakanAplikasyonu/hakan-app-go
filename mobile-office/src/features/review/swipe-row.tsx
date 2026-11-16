import { useMemo, useState, type ReactNode } from 'react';
import { Animated, PanResponder, StyleSheet, View } from 'react-native';

import { Text } from '@/components/ui';
import { useColors } from '@/core/theme';

import { swipeAction, type SwipeAction } from './format';

/**
 * Kaydırılabilir satır: sağa kaydır → onayla, sola kaydır → reddet.
 * Yatay hareket dikey kaydırmadan baskın olduğunda devreye girer; eşik
 * aşılmazsa satır yerine döner. `enabled` false ise hareket yoktur.
 */
export function SwipeRow({
  children,
  width,
  enabled,
  onAction,
}: {
  children: ReactNode;
  width: number;
  enabled: boolean;
  onAction: (a: Exclude<SwipeAction, null>) => void;
}) {
  const c = useColors();
  const [x] = useState(() => new Animated.Value(0));
  const responder = useMemo(
    () =>
      PanResponder.create({
        onMoveShouldSetPanResponder: (_, g) => enabled && Math.abs(g.dx) > 12 && Math.abs(g.dx) > Math.abs(g.dy) * 2,
        onPanResponderMove: Animated.event([null, { dx: x }], { useNativeDriver: false }),
        onPanResponderRelease: (_, g) => {
          const a = swipeAction(g.dx, width);
          Animated.spring(x, { toValue: 0, useNativeDriver: false, bounciness: 4 }).start();
          if (a) onAction(a);
        },
        onPanResponderTerminate: () => Animated.spring(x, { toValue: 0, useNativeDriver: false }).start(),
      }),
    [enabled, onAction, width, x],
  );
  return (
    <View style={styles.wrap}>
      <View style={[StyleSheet.absoluteFill, styles.under]}>
        <Text style={{ color: c.success, fontWeight: '700' }}>Onayla</Text>
        <Text style={{ color: c.danger, fontWeight: '700' }}>Reddet</Text>
      </View>
      <Animated.View style={{ transform: [{ translateX: x }] }} {...responder.panHandlers}>
        {children}
      </Animated.View>
    </View>
  );
}

const styles = StyleSheet.create({
  wrap: { borderRadius: 16, overflow: 'hidden' },
  under: { flexDirection: 'row', justifyContent: 'space-between', alignItems: 'center', paddingHorizontal: 20 },
});

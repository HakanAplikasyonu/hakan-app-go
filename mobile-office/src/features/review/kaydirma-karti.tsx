import { useEffect, type ReactNode } from 'react';
import { StyleSheet, Text, View, type ViewStyle } from 'react-native';
import { Gesture, GestureDetector } from 'react-native-gesture-handler';
import Animated, {
  Extrapolation,
  interpolate,
  runOnJS,
  useAnimatedStyle,
  useReducedMotion,
  useSharedValue,
  withSpring,
  withTiming,
} from 'react-native-reanimated';

import { cardTilt, swipeAction, type SwipeAction } from './format';

/** Tasarım: hareket 320 ms, yay damping 18 (docs/tasarim/README.md, "Hareket"). */
const SPRING = { damping: 18, stiffness: 220 } as const;
const SLOW_MS = 320;

export type KaydirmaRenkleri = { ok: string; onOk: string; danger: string; onDanger: string; brand: string; onBrand: string };

/**
 * Kaydırarak onay kartı (bilesenler.md, `KaydirmaKarti`): sağa onay, sola red,
 * yukarı düzelt. Eşik kart genişliğinin %35'i; eşik aşılmazsa kart yay ile
 * yerine döner. Alt düğmeler zorunlu, bu bileşen yalnızca hareketi taşır.
 * "Hareketi azalt" açıksa kart kaymaz; karar verilince yalnızca opaklık geçişi.
 */
export function KaydirmaKarti({
  children,
  cardKey,
  width,
  height,
  enabled,
  colors,
  onApprove,
  onReject,
  onEdit,
  style,
  testID,
}: {
  children: ReactNode;
  /** Kartın kimliği; değişince yeni kart ortada ve görünür başlar. */
  cardKey: string;
  width: number;
  height: number;
  enabled: boolean;
  colors: KaydirmaRenkleri;
  onApprove: () => void;
  onReject: () => void;
  onEdit: () => void;
  style?: ViewStyle;
  testID?: string;
}) {
  const reduce = useReducedMotion();
  const x = useSharedValue(0);
  const y = useSharedValue(0);
  const fade = useSharedValue(1);

  useEffect(() => {
    x.set(0);
    y.set(0);
    fade.set(reduce ? withTiming(1, { duration: SLOW_MS }) : 1);
  }, [cardKey, fade, reduce, x, y]);

  const fire = (a: Exclude<SwipeAction, null>) => {
    if (a === 'accept') onApprove();
    else if (a === 'reject') onReject();
    else onEdit();
  };

  const pan = Gesture.Pan()
    .enabled(enabled)
    .minDistance(8)
    .onUpdate((e) => {
      if (reduce) return;
      x.set(e.translationX);
      y.set(Math.min(0, e.translationY));
    })
    .onEnd((e) => {
      const a = swipeAction(e.translationX, width, 0.35, e.translationY, height);
      if (a === 'accept' || a === 'reject') {
        if (reduce) {
          fade.set(withTiming(0, { duration: SLOW_MS }));
        } else {
          x.set(withTiming((a === 'accept' ? 1.4 : -1.4) * width, { duration: SLOW_MS }));
        }
        runOnJS(fire)(a);
        return;
      }
      x.set(withSpring(0, SPRING));
      y.set(withSpring(0, SPRING));
      if (a === 'correct') runOnJS(fire)(a);
    });

  const card = useAnimatedStyle(() => ({
    opacity: fade.value,
    transform: [{ translateX: x.value }, { translateY: y.value }, { rotate: `${cardTilt(x.value, width)}deg` }],
  }));
  const approveHint = useAnimatedStyle(() => ({
    opacity: interpolate(x.value, [0, width * 0.35], [0, 1], Extrapolation.CLAMP),
  }));
  const rejectHint = useAnimatedStyle(() => ({
    opacity: interpolate(x.value, [-width * 0.35, 0], [1, 0], Extrapolation.CLAMP),
  }));
  const editHint = useAnimatedStyle(() => ({
    opacity: interpolate(y.value, [-Math.max(80, height * 0.25), 0], [1, 0], Extrapolation.CLAMP),
  }));

  return (
    <View style={[{ height }, style]} testID={testID}>
      <Animated.View pointerEvents="none" style={[styles.under, { backgroundColor: colors.ok, justifyContent: 'flex-start' }, approveHint]}>
        <Text style={[styles.hint, { color: colors.onOk }]}>✓ Onayla</Text>
      </Animated.View>
      <Animated.View pointerEvents="none" style={[styles.under, { backgroundColor: colors.danger, justifyContent: 'flex-end' }, rejectHint]}>
        <Text style={[styles.hint, { color: colors.onDanger }]}>Reddet ✕</Text>
      </Animated.View>
      <Animated.View pointerEvents="none" style={[styles.under, { backgroundColor: colors.brand, justifyContent: 'center', alignItems: 'flex-end' }, editHint]}>
        <Text style={[styles.hint, { color: colors.onBrand, paddingBottom: 24 }]}>Düzelt</Text>
      </Animated.View>
      <GestureDetector gesture={pan}>
        <Animated.View style={[StyleSheet.absoluteFill, card]}>{children}</Animated.View>
      </GestureDetector>
    </View>
  );
}

const styles = StyleSheet.create({
  under: { position: 'absolute', top: 0, left: 0, right: 0, bottom: 0, borderRadius: 22, flexDirection: 'row', alignItems: 'center', paddingHorizontal: 28 },
  hint: { fontSize: 17, fontWeight: '700' },
});

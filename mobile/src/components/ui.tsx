import type { ComponentRef, ReactNode, Ref } from 'react';
import {
  ActivityIndicator,
  Pressable,
  ScrollView,
  StyleSheet,
  Text as RNText,
  TextInput,
  View,
  type TextInputProps,
  type TextProps,
  type ViewStyle,
} from 'react-native';
import { SafeAreaView } from 'react-native-safe-area-context';

import { useColors } from '@/core/theme';

export function Screen({ children, scroll = true, style }: { children: ReactNode; scroll?: boolean; style?: ViewStyle }) {
  const c = useColors();
  const body = <View style={[styles.screenBody, style]}>{children}</View>;
  return (
    <SafeAreaView style={{ flex: 1, backgroundColor: c.background }} edges={['top', 'left', 'right', 'bottom']}>
      {scroll ? (
        <ScrollView contentContainerStyle={{ flexGrow: 1 }} keyboardShouldPersistTaps="handled">
          {body}
        </ScrollView>
      ) : (
        body
      )}
    </SafeAreaView>
  );
}

type Tone = 'default' | 'muted' | 'danger' | 'brand';

export function Text({ tone = 'default', variant = 'body', style, ...rest }: TextProps & { tone?: Tone; variant?: 'title' | 'heading' | 'body' | 'caption' }) {
  const c = useColors();
  const color = tone === 'muted' ? c.muted : tone === 'danger' ? c.danger : tone === 'brand' ? c.brand : c.text;
  return <RNText style={[styles[variant], { color }, style]} {...rest} />;
}

export function Card({ children, style }: { children: ReactNode; style?: ViewStyle }) {
  const c = useColors();
  return <View style={[styles.card, { backgroundColor: c.surface, borderColor: c.border }, style]}>{children}</View>;
}

export function Button({
  title,
  onPress,
  loading,
  disabled,
  variant = 'primary',
}: {
  title: string;
  onPress: () => void;
  loading?: boolean;
  disabled?: boolean;
  variant?: 'primary' | 'secondary' | 'ghost';
}) {
  const c = useColors();
  const bg = variant === 'primary' ? c.primary : variant === 'secondary' ? c.surface : 'transparent';
  const fg = variant === 'primary' ? c.primaryText : c.text;
  const off = disabled || loading;
  return (
    <Pressable
      accessibilityRole="button"
      accessibilityState={{ disabled: !!off, busy: !!loading }}
      onPress={onPress}
      disabled={off}
      style={({ pressed }) => [
        styles.button,
        { backgroundColor: bg, borderColor: variant === 'secondary' ? c.border : 'transparent', opacity: off ? 0.5 : pressed ? 0.85 : 1 },
      ]}>
      {loading ? <ActivityIndicator color={fg} /> : <RNText style={[styles.buttonText, { color: fg }]}>{title}</RNText>}
    </Pressable>
  );
}

export type InputRef = ComponentRef<typeof TextInput>;

export function Field({
  label,
  error,
  style,
  inputRef,
  ...rest
}: TextInputProps & { label: string; error?: string | null; inputRef?: Ref<InputRef> }) {
  const c = useColors();
  return (
    <View style={{ gap: 6 }}>
      <Text variant="caption" tone="muted">
        {label}
      </Text>
      <TextInput
        ref={inputRef}
        placeholderTextColor={c.muted}
        style={[styles.input, { color: c.text, backgroundColor: c.surface, borderColor: error ? c.danger : c.border }, style]}
        {...rest}
      />
      {error ? (
        <Text variant="caption" tone="danger">
          {error}
        </Text>
      ) : null}
    </View>
  );
}

export function Notice({ children, tone = 'danger' }: { children: ReactNode; tone?: 'danger' | 'info' }) {
  const c = useColors();
  const color = tone === 'danger' ? c.danger : c.brand;
  return (
    <View accessibilityRole="alert" style={[styles.notice, { borderColor: color, backgroundColor: c.surface }]}>
      <Text style={{ color }}>{children}</Text>
    </View>
  );
}

const styles = StyleSheet.create({
  screenBody: { flex: 1, padding: 20, gap: 16 },
  title: { fontSize: 28, fontWeight: '700', letterSpacing: -0.5 },
  heading: { fontSize: 18, fontWeight: '600' },
  body: { fontSize: 16, lineHeight: 22 },
  caption: { fontSize: 13, lineHeight: 18 },
  card: { borderWidth: StyleSheet.hairlineWidth, borderRadius: 16, padding: 16, gap: 8 },
  button: { minHeight: 48, borderRadius: 12, borderWidth: 1, alignItems: 'center', justifyContent: 'center', paddingHorizontal: 16 },
  buttonText: { fontSize: 16, fontWeight: '600' },
  input: { minHeight: 48, borderWidth: 1, borderRadius: 12, paddingHorizontal: 14, fontSize: 16 },
  notice: { borderWidth: 1, borderRadius: 12, padding: 12 },
});

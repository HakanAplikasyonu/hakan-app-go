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

import { tokens, useColors } from './theme.ts';

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
  // Arayüz v1 (#324): tokens.ts `type` ölçüleri; mobil gövde 15/22.
  title: { fontFamily: tokens.font.display, fontSize: tokens.type.h1.size, lineHeight: tokens.type.h1.line, letterSpacing: -0.5 },
  heading: { fontFamily: 'InterTight_600SemiBold', fontSize: 18, lineHeight: 24 },
  body: { fontFamily: tokens.font.sans, fontSize: tokens.type.body.size, lineHeight: tokens.type.body.line },
  caption: { fontFamily: tokens.font.sans, fontSize: tokens.type.small.size, lineHeight: tokens.type.small.line },
  card: { borderWidth: StyleSheet.hairlineWidth, borderRadius: tokens.radius.lg, padding: 16, gap: 8 },
  button: { minHeight: 46, borderRadius: 12, borderWidth: 1, alignItems: 'center', justifyContent: 'center', paddingHorizontal: 16 },
  buttonText: { fontFamily: tokens.font.sansSemi, fontSize: 15 },
  input: { minHeight: 46, borderWidth: 1, borderRadius: tokens.radius.md, paddingHorizontal: 14, fontSize: 15, fontFamily: tokens.font.sans },
  notice: { borderWidth: 1, borderRadius: 12, padding: 12 },
});

export type ChipTone = 'neutral' | 'brand' | 'success' | 'warning' | 'danger';

export function Chip({ label, tone = 'neutral' }: { label: string; tone?: ChipTone }) {
  const c = useColors();
  const color = { neutral: c.muted, brand: c.brand, success: c.success, warning: c.warning, danger: c.danger }[tone];
  return (
    <View style={[chipStyles.chip, { borderColor: color }]}>
      <RNText style={[chipStyles.text, { color }]}>{label}</RNText>
    </View>
  );
}

/** Tappable list row with an optional leading element. */
export function Row({
  title,
  subtitle,
  leading,
  trailing,
  onPress,
  accessibilityLabel,
}: {
  title: string;
  subtitle?: string;
  leading?: ReactNode;
  trailing?: ReactNode;
  onPress?: () => void;
  accessibilityLabel?: string;
}) {
  const c = useColors();
  return (
    <Pressable
      accessibilityRole={onPress ? 'button' : undefined}
      accessibilityLabel={accessibilityLabel}
      onPress={onPress}
      disabled={!onPress}
      style={({ pressed }) => [chipStyles.row, { backgroundColor: c.surface, borderColor: c.border, opacity: pressed ? 0.85 : 1 }]}>
      {leading}
      <View style={{ flex: 1, gap: 2 }}>
        <Text style={{ fontFamily: tokens.font.sansSemi }} numberOfLines={2}>
          {title}
        </Text>
        {subtitle ? (
          <Text variant="caption" tone="muted" numberOfLines={2}>
            {subtitle}
          </Text>
        ) : null}
      </View>
      {trailing}
    </Pressable>
  );
}

const chipStyles = StyleSheet.create({
  chip: { borderWidth: 1, borderRadius: 999, paddingHorizontal: 10, paddingVertical: 3, alignSelf: 'flex-start' },
  text: { fontSize: 12, fontFamily: tokens.font.sansSemi },
  row: { flexDirection: 'row', alignItems: 'center', gap: 12, borderWidth: StyleSheet.hairlineWidth, borderRadius: 16, padding: 12 },
});

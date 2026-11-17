import { ScrollView, Pressable, StyleSheet } from 'react-native';

import { Text } from '@/components/ui';
import { useColors } from '@/core/theme';

/** Yatay dönem seçici (#393): seçili dönem marka renginde. */
export function PeriodChips({
  periods,
  selected,
  onSelect,
  label,
  testIDPrefix,
}: {
  periods: readonly { key: string; label: string }[];
  selected: string | undefined;
  onSelect: (key: string) => void;
  label: (p: { key: string; label: string }) => string;
  testIDPrefix: string;
}) {
  const c = useColors();
  return (
    <ScrollView horizontal showsHorizontalScrollIndicator={false} contentContainerStyle={styles.row} accessibilityRole="tablist">
      {periods.map((p) => {
        const on = p.key === selected;
        return (
          <Pressable
            key={p.key}
            testID={`${testIDPrefix}-${p.key}`}
            accessibilityRole="tab"
            accessibilityState={{ selected: on }}
            accessibilityLabel={label(p)}
            onPress={() => onSelect(p.key)}
            style={[styles.chip, { borderColor: on ? c.brand : c.border, backgroundColor: on ? c.brand : c.surface }]}>
            <Text variant="caption" style={{ color: on ? c.background : c.text, fontWeight: '600' }}>
              {p.label}
            </Text>
          </Pressable>
        );
      })}
    </ScrollView>
  );
}

const styles = StyleSheet.create({
  row: { gap: 8, paddingVertical: 2 },
  chip: { minHeight: 36, borderRadius: 999, borderWidth: 1, paddingHorizontal: 14, justifyContent: 'center' },
});

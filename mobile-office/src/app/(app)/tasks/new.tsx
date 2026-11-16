import { useLocalSearchParams, useRouter } from 'expo-router';
import { useState } from 'react';
import { Pressable, StyleSheet, View } from 'react-native';

import { Button, Field, Notice, Screen, Text } from '@/components/ui';
import { errorMessage } from '@/core/api/envelope';
import { useColors } from '@/core/theme';
import { useCreateRequest } from '@/features/doctasks/api';
import { commonRequests, dayLabel, dueChoices, dueDateFromDays, validateRequest } from '@/features/doctasks/format';

/** Mükellefe belge talebi aç (#240): ne istendiği, açıklama, son tarih. */
export default function NewRequestScreen() {
  const { engagement, title: taxpayer } = useLocalSearchParams<{ engagement: string; title?: string }>();
  const router = useRouter();
  const c = useColors();
  const create = useCreateRequest(engagement);
  const [title, setTitle] = useState('');
  const [note, setNote] = useState('');
  const [days, setDays] = useState<number>(7);
  const [touched, setTouched] = useState(false);
  const invalid = validateRequest({ title, note, dueDays: days });

  async function submit() {
    setTouched(true);
    if (invalid) return;
    try {
      const t = await create.mutateAsync({ title: title.trim(), note: note.trim() || undefined, due_date: dueDateFromDays(days) });
      router.replace({ pathname: '/tasks/[uuid]', params: { uuid: t.uuid, engagement } });
    } catch {
      // create.error gösterilir
    }
  }

  return (
    <Screen>
      {taxpayer ? (
        <Text tone="muted" numberOfLines={2}>
          {taxpayer}
        </Text>
      ) : null}
      <Field label="İstenen belge" value={title} onChangeText={setTitle} placeholder="Örn. Kira kontratı" maxLength={200} testID="request-title" />
      <View style={styles.chips}>
        {commonRequests.map((r) => (
          <Pressable key={r} onPress={() => setTitle(r)} style={[styles.chip, { borderColor: c.border, backgroundColor: c.surface }]} accessibilityRole="button">
            <Text variant="caption">{r}</Text>
          </Pressable>
        ))}
      </View>
      <Field label="Açıklama (isteğe bağlı)" value={note} onChangeText={setNote} multiline style={{ minHeight: 80 }} maxLength={1000} />
      <Text variant="caption" tone="muted">
        Son tarih
      </Text>
      <View style={styles.chips} accessibilityRole="radiogroup">
        {dueChoices.map((d) => (
          <Pressable
            key={d}
            accessibilityRole="radio"
            accessibilityState={{ selected: days === d }}
            onPress={() => setDays(d)}
            style={[styles.chip, { borderColor: days === d ? c.brand : c.border, backgroundColor: c.surface }]}>
            <Text variant="caption" tone={days === d ? 'brand' : undefined}>
              {d} gün
            </Text>
          </Pressable>
        ))}
      </View>
      <Text variant="caption" tone="muted">
        {dayLabel(dueDateFromDays(days))} gün sonuna kadar. Mükellefe bildirim gider; yüklemediği sürece hatırlatılır.
      </Text>
      {touched && invalid ? <Notice>{invalid}</Notice> : null}
      {create.error ? <Notice>{errorMessage(create.error)}</Notice> : null}
      <Button title="Talebi gönder" onPress={() => void submit()} loading={create.isPending} />
    </Screen>
  );
}

const styles = StyleSheet.create({
  chips: { flexDirection: 'row', flexWrap: 'wrap', gap: 8 },
  chip: { borderWidth: 1, borderRadius: 999, paddingHorizontal: 12, paddingVertical: 6 },
});

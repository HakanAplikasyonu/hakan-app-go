import { useLocalSearchParams, useRouter } from 'expo-router';
import { useState } from 'react';
import { ActivityIndicator, FlatList, Pressable, RefreshControl, StyleSheet, View } from 'react-native';

import { Button, Chip, Notice, Row, Text } from '@/components/ui';
import { errorMessage } from '@/core/api/envelope';
import { useAuth } from '@/core/auth/auth-provider';
import { useColors } from '@/core/theme';
import { useTasks } from '@/features/doctasks/api';
import { dayLabel, statusLabel, statusTone, taskTitle, type TaskStatus } from '@/features/doctasks/format';

const filters: { key: string; label: string }[] = [
  { key: 'open', label: 'Bekleyen' },
  { key: 'submitted', label: 'Yüklenen' },
  { key: 'matched', label: 'Tamamlanan' },
  { key: 'all', label: 'Tümü' },
];

/** Bir mükellefin belge görevleri (#240). */
export default function TasksScreen() {
  const { engagement, title } = useLocalSearchParams<{ engagement: string; title?: string }>();
  const c = useColors();
  const router = useRouter();
  const { can } = useAuth();
  const [status, setStatus] = useState('open');
  const q = useTasks(engagement, status);

  return (
    <View style={{ flex: 1, backgroundColor: c.background }}>
      <View style={styles.filters} accessibilityRole="tablist">
        {filters.map((f) => (
          <Pressable
            key={f.key}
            accessibilityRole="tab"
            accessibilityState={{ selected: status === f.key }}
            onPress={() => setStatus(f.key)}
            style={[styles.filter, { borderColor: status === f.key ? c.brand : c.border, backgroundColor: c.surface }]}>
            <Text variant="caption" tone={status === f.key ? 'brand' : 'muted'} style={{ fontWeight: '600' }}>
              {f.label}
            </Text>
          </Pressable>
        ))}
      </View>
      {q.error ? (
        <View style={{ padding: 16 }}>
          <Notice>{errorMessage(q.error)}</Notice>
        </View>
      ) : null}
      <FlatList
        data={q.data?.items ?? []}
        keyExtractor={(t) => t.uuid}
        contentContainerStyle={{ padding: 16, gap: 8 }}
        refreshControl={<RefreshControl refreshing={q.isRefetching} onRefresh={() => void q.refetch()} tintColor={c.brand} />}
        ListEmptyComponent={q.isLoading ? <ActivityIndicator color={c.brand} /> : <Text tone="muted">Görev yok.</Text>}
        renderItem={({ item: t }) => (
          <Row
            title={taskTitle(t)}
            subtitle={`Son gün ${dayLabel(t.due_at)}${t.overdue ? ' · gecikti' : ''}${t.upload_count ? ` · ${t.upload_count} dosya` : ''}`}
            trailing={<Chip tone={t.overdue ? 'danger' : statusTone(t.status as TaskStatus)} label={statusLabel[t.status as TaskStatus] ?? t.status} />}
            onPress={() => router.push({ pathname: '/tasks/[uuid]', params: { uuid: t.uuid, engagement } })}
          />
        )}
      />
      {can('tenant.doc_tasks.write') ? (
        <View style={{ padding: 16 }}>
          <Button title="Yeni belge iste" onPress={() => router.push({ pathname: '/tasks/new', params: { engagement, title: title ?? '' } })} />
        </View>
      ) : null}
    </View>
  );
}

const styles = StyleSheet.create({
  filters: { flexDirection: 'row', gap: 8, paddingHorizontal: 16, paddingTop: 12, flexWrap: 'wrap' },
  filter: { borderWidth: 1, borderRadius: 999, paddingHorizontal: 12, paddingVertical: 6 },
});

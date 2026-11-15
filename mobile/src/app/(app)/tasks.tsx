import { RefreshControl, ScrollView, View } from 'react-native';
import { SafeAreaView } from 'react-native-safe-area-context';

import { Card, Chip, Notice, Row, Text } from '@/components/ui';
import { errorMessage } from '@/core/api/envelope';
import { useColors } from '@/core/theme';
import { openTask, useTasks } from '@/features/tasks/use-tasks';

const dueLabel = (d: string) => {
  const [y, m, day] = d.split('-').map(Number);
  return new Intl.DateTimeFormat('tr-TR', { day: 'numeric', month: 'long', timeZone: 'UTC' }).format(new Date(Date.UTC(y, m - 1, day)));
};

/** Görevler: müşavirin istediği eksik belgeler + telefondaki fişlerden çıkan işler. */
export default function TasksScreen() {
  const { tasks, serverAvailable, query } = useTasks();
  const c = useColors();
  return (
    <SafeAreaView style={{ flex: 1, backgroundColor: c.background }} edges={['left', 'right', 'bottom']}>
      <ScrollView
        contentContainerStyle={{ padding: 20, gap: 10, maxWidth: 640, width: '100%', alignSelf: 'center' }}
        refreshControl={<RefreshControl refreshing={query.isFetching} onRefresh={() => void query.refetch()} />}>
        {query.isError ? <Notice>{errorMessage(query.error)}</Notice> : null}
        {!query.isLoading && !serverAvailable ? (
          <Notice tone="info">Müşavirinizin eksik belge istekleri yakında burada görünecek. Şimdilik telefondaki fişlerinizle ilgili işler listeleniyor.</Notice>
        ) : null}
        {tasks.length === 0 && !query.isLoading ? (
          <Card>
            <Text variant="heading">Yapılacak iş yok</Text>
            <Text tone="muted">Müşaviriniz bir belge istediğinde burada görürsünüz.</Text>
          </Card>
        ) : null}
        {tasks.map((t) => (
          <Row
            key={t.id}
            title={t.title}
            subtitle={t.detail || undefined}
            onPress={() => openTask(t)}
            trailing={
              <View style={{ alignItems: 'flex-end', gap: 4 }}>
                <Chip label={t.source === 'server' ? 'Müşavir' : 'Fiş'} tone={t.source === 'server' ? 'brand' : 'warning'} />
                {t.due ? (
                  <Text variant="caption" tone="muted">
                    {dueLabel(t.due)}
                  </Text>
                ) : null}
              </View>
            }
          />
        ))}
      </ScrollView>
    </SafeAreaView>
  );
}

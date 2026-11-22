import { BosDurum, KristalYukleyici, TacRozeti } from '@hakan/mobile-core/hakan';
import { Ikon } from '@hakan/mobile-core/ui/ikon';
import { yerlesim } from '@hakan/mobile-core/ui/layout';
import { EkranBasligi, Satir, Segment } from '@hakan/mobile-core/ui/parcalar';
import { useTheme } from '@hakan/mobile-core/ui/theme';
import { useState } from 'react';
import { FlatList, RefreshControl, StyleSheet, useWindowDimensions, View } from 'react-native';
import { SafeAreaView } from 'react-native-safe-area-context';

import { Notice } from '@/components/ui';
import { errorMessage } from '@/core/api/envelope';
import { useAuth } from '@/core/auth/auth-provider';
import { dueLine, taskCrown } from '@/features/home/crown';
import { IstekAyrintisi } from '@/features/tasks/istek-ayrintisi';
import { donemEtiketi, taskTitle, type Task } from '@/features/tasks/tasks';
import { openTask, useTasks } from '@/features/tasks/use-tasks';

type Sekme = 'acik' | 'tamam';

/**
 * İstekler: müşavirin istediği belgeler ve telefondaki fişlerden çıkan işler.
 * Tablette (MukellefTablet) solda liste, sağda seçili isteğin ayrıntısı.
 */
export default function TasksScreen() {
  const { colors: c } = useTheme();
  const { width } = useWindowDimensions();
  const tablet = yerlesim(width) !== 'telefon';
  const { taxpayer } = useAuth();
  const { tasks, query } = useTasks();
  const [sekme, setSekme] = useState<Sekme>('acik');
  const [secili, setSecili] = useState<string | null>(null);

  const done: Task[] = (query.data ?? [])
    .filter((t) => t.status === 'matched' || t.status === 'cancelled')
    .map((t) => ({ id: `server:${t.uuid}`, source: 'server', title: taskTitle(t), detail: '', action: { type: 'task', uuid: t.uuid }, due: null }));
  const list = sekme === 'acik' ? tasks : done;
  const serverUuid = (t: Task) => (t.action.type === 'task' ? t.action.uuid : null);
  const firstServer = list.map(serverUuid).find(Boolean) ?? null;
  const detail = tablet ? (secili && list.some((t) => serverUuid(t) === secili) ? secili : firstServer) : null;

  const listEl = (
    <FlatList
      data={list}
      keyExtractor={(t) => t.id}
      refreshControl={<RefreshControl refreshing={query.isRefetching} onRefresh={() => void query.refetch()} tintColor={c.brandText} />}
      ListHeaderComponent={query.isError ? <Notice>{errorMessage(query.error)}</Notice> : undefined}
      ListEmptyComponent={
        query.isLoading ? (
          <View style={{ alignItems: 'center', paddingVertical: 40 }}>
            <KristalYukleyici etiket="İstekler yükleniyor" />
          </View>
        ) : (
          <BosDurum
            baslik={sekme === 'acik' ? 'Yapılacak iş yok' : 'Tamamlanan istek yok'}
            aciklama={sekme === 'acik' ? 'Müşaviriniz bir belge istediğinde burada görürsünüz.' : undefined}
          />
        )
      }
      renderItem={({ item: t, index }) => {
        const server = query.data?.find((x) => `server:${x.uuid}` === t.id);
        const due = server && sekme === 'acik' ? dueLine(server.due_at) : null;
        const uuid = serverUuid(t);
        return (
          <Satir
            testID="task-row"
            ilk={index === 0}
            vurgulu={tablet && !!uuid && uuid === detail}
            sol={t.source === 'server' ? <TacRozeti durum={server ? taskCrown(server.status, server.overdue) : 'bekleniyor'} /> : <Ikon ad="uyari" renk={c.warn} />}
            baslik={t.title}
            alt={[server?.period ? donemEtiketi(server.period) : '', due?.text, t.detail.split('\n')[0]].filter(Boolean).join(' · ') || undefined}
            altRenk={due?.late ? c.danger : undefined}
            sag={tablet ? undefined : <Ikon ad="sag" boyut={16} renk={c.text3} />}
            onPress={() => (tablet && uuid ? setSecili(uuid) : openTask(t))}
          />
        );
      }}
    />
  );

  return (
    <SafeAreaView style={{ flex: 1, backgroundColor: c.bg }} edges={['top', 'left', 'right']}>
      <View style={{ flex: 1, flexDirection: tablet ? 'row' : 'column' }}>
        <View style={tablet ? [styles.listPane, { backgroundColor: c.surface, borderRightColor: c.line }] : { flex: 1 }}>
          <EkranBasligi ust={taxpayer?.taxpayer.title} baslik="İstekler" />
          <Segment
            testIDOnek="tasks-tab"
            secenekler={[
              { key: 'acik', label: `Açık ${tasks.length || ''}`.trim() },
              { key: 'tamam', label: `Tamamlanan ${done.length || ''}`.trim() },
            ]}
            secili={sekme}
            onSec={setSekme}
            style={{ marginHorizontal: 16, marginBottom: 10 }}
          />
          {listEl}
        </View>
        {tablet ? (
          <View style={{ flex: 1 }}>{detail ? <IstekAyrintisi key={detail} uuid={detail} genis /> : <BosDurum baslik="Bir istek seçin" style={{ flex: 1, justifyContent: 'center' }} />}</View>
        ) : null}
      </View>
    </SafeAreaView>
  );
}

const styles = StyleSheet.create({
  listPane: { width: 400, borderRightWidth: StyleSheet.hairlineWidth },
});

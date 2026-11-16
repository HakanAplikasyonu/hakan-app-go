import { useRouter } from 'expo-router';
import { useEffect, useState } from 'react';
import { ActivityIndicator, Pressable, RefreshControl, ScrollView, StyleSheet, View } from 'react-native';
import { SafeAreaView } from 'react-native-safe-area-context';

import { BrandMark } from '@/components/brand-mark';
import { Button, Card, Chip, Notice, Row, Text } from '@/components/ui';
import { errorMessage } from '@/core/api/envelope';
import { useAuth } from '@/core/auth/auth-provider';
import { enablePush, getPushPermission, type PushPermission } from '@/core/push/push';
import { useColors } from '@/core/theme';
import { useDashboard, type DashboardUpload } from '@/features/dashboard/api';
import { clock, countLabel, greeting, uploadKindLabel, type UploadKind } from '@/features/dashboard/format';

/** Ana pano (#237): inceleme kuyruğu, belge görevleri, aktif uyarılar, bugünkü yüklemeler. */
export default function HomeScreen() {
  const { me, office, offices, switchOffice, signOut } = useAuth();
  const c = useColors();
  const router = useRouter();
  const q = useDashboard();
  const d = q.data;
  const [push, setPush] = useState<PushPermission | null>(null);
  useEffect(() => {
    getPushPermission()
      .then(setPush)
      .catch(() => setPush('unavailable'));
  }, []);

  return (
    <SafeAreaView style={{ flex: 1, backgroundColor: c.background }} edges={['top', 'left', 'right']}>
      <ScrollView
        contentContainerStyle={styles.body}
        refreshControl={<RefreshControl refreshing={q.isRefetching} onRefresh={() => void q.refetch()} tintColor={c.brand} />}>
        <View style={{ flexDirection: 'row', alignItems: 'center', gap: 12 }}>
          <BrandMark size={40} />
          <View style={{ flex: 1 }}>
            <Text variant="heading" numberOfLines={1}>
              {office?.organization.name}
            </Text>
            <Text variant="caption" tone="muted">
              {greeting()}
              {me?.user.name ? `, ${me.user.name}` : ''}
            </Text>
          </View>
        </View>

        {push === 'undetermined' ? (
          <Card>
            <Text variant="heading">Bildirimleri açın</Text>
            <Text tone="muted">Mükellef belge yüklediğinde ve kritik uyarı açıldığında telefonunuza haber verelim.</Text>
            <Button
              title="Bildirimleri aç"
              onPress={() =>
                void enablePush().then((r) => setPush(r === 'registered' ? 'granted' : r === 'denied' ? 'denied' : 'unavailable'))
              }
            />
          </Card>
        ) : null}

        {q.isLoading ? <ActivityIndicator color={c.brand} style={{ marginTop: 32 }} /> : null}
        {q.error ? <Notice>{errorMessage(q.error)}</Notice> : null}

        {d ? (
          <>
            <View style={styles.grid}>
              <Stat
                testID="stat-review"
                label="İnceleme kuyruğu"
                value={d.review ? countLabel(d.review.pending) : '—'}
                hint={d.review ? 'onay bekleyen öneri' : 'yetkiniz yok'}
                onPress={d.review ? () => router.push('/review') : undefined}
              />
              <Stat
                testID="stat-tasks"
                label="Belge görevleri"
                value={d.doc_tasks ? countLabel(d.doc_tasks.open + d.doc_tasks.submitted) : '—'}
                hint={d.doc_tasks ? (d.doc_tasks.overdue ? `${d.doc_tasks.overdue} gecikmiş` : 'açık görev') : 'yetkiniz yok'}
                tone={d.doc_tasks?.overdue ? 'danger' : undefined}
              />
              <Stat
                testID="stat-warnings"
                label="Aktif uyarılar"
                value={d.warnings ? countLabel(d.warnings.open) : '—'}
                hint={d.warnings ? (d.warnings.blocker ? `${d.warnings.blocker} engelleyici` : 'açık uyarı') : 'yetkiniz yok'}
                tone={d.warnings?.blocker ? 'danger' : undefined}
                onPress={d.warnings ? () => router.push('/warnings') : undefined}
              />
              <Stat testID="stat-uploads" label="Bugün gelen" value={countLabel(d.uploads.total)} hint="yükleme" />
            </View>

            {d.warnings && d.warnings.open > 0 ? (
              <Card>
                <View style={{ flexDirection: 'row', gap: 8, flexWrap: 'wrap' }}>
                  {d.warnings.blocker ? <Chip tone="danger" label={`${d.warnings.blocker} engelleyici`} /> : null}
                  {d.warnings.warning ? <Chip tone="warning" label={`${d.warnings.warning} uyarı`} /> : null}
                  {d.warnings.info ? <Chip tone="brand" label={`${d.warnings.info} bilgi`} /> : null}
                </View>
                <Button title="Uyarıları aç" variant="secondary" onPress={() => router.push('/warnings')} />
              </Card>
            ) : null}

            <Button title="Mükellefler" variant="secondary" onPress={() => router.push('/clients')} />

            <Text variant="heading">Bugün gelen yüklemeler</Text>
            {d.uploads.recent.length === 0 ? (
              <Text tone="muted">Bugün henüz yükleme yok.</Text>
            ) : (
              <View style={{ gap: 8 }}>
                {d.uploads.recent.map((u) => (
                  <UploadRow key={`${u.kind}-${u.uuid}`} upload={u} />
                ))}
                {d.uploads.total > d.uploads.recent.length ? (
                  <Text variant="caption" tone="muted">
                    Son {d.uploads.recent.length} yükleme gösteriliyor ({d.uploads.total} toplam).
                  </Text>
                ) : null}
              </View>
            )}
          </>
        ) : null}

        <View style={{ height: 16 }} />
        {offices.length > 1 ? <Button title="Ofis değiştir" variant="secondary" onPress={switchOffice} /> : null}
        <Button title="Çıkış yap" variant="ghost" onPress={signOut} />
      </ScrollView>
    </SafeAreaView>
  );
}

function Stat({
  label,
  value,
  hint,
  tone,
  onPress,
  testID,
}: {
  label: string;
  value: string;
  hint: string;
  tone?: 'danger';
  onPress?: () => void;
  testID?: string;
}) {
  const c = useColors();
  return (
    <Pressable
      testID={testID}
      accessibilityRole={onPress ? 'button' : 'summary'}
      accessibilityLabel={`${label}: ${value}, ${hint}`}
      onPress={onPress}
      disabled={!onPress}
      style={({ pressed }) => [styles.stat, { backgroundColor: c.surface, borderColor: c.border, opacity: pressed ? 0.85 : 1 }]}>
      <Text variant="caption" tone="muted">
        {label}
      </Text>
      <Text variant="title" tone={tone}>
        {value}
      </Text>
      <Text variant="caption" tone={tone ?? 'muted'}>
        {hint}
      </Text>
    </Pressable>
  );
}

function UploadRow({ upload }: { upload: DashboardUpload }) {
  const router = useRouter();
  return (
    <Row
      onPress={() =>
        router.push({ pathname: '/clients/[engagement]', params: { engagement: upload.engagement_uuid, title: upload.taxpayer_title } })
      }
      trailing={<Text tone="muted">›</Text>}
      title={upload.taxpayer_title}
      subtitle={`${uploadKindLabel[upload.kind as UploadKind] ?? upload.kind} · ${clock(upload.created_at)}`}
      accessibilityLabel={`${upload.taxpayer_title}, ${uploadKindLabel[upload.kind as UploadKind] ?? upload.kind}`}
    />
  );
}

const styles = StyleSheet.create({
  body: { padding: 20, gap: 16, flexGrow: 1 },
  grid: { flexDirection: 'row', flexWrap: 'wrap', gap: 12 },
  stat: { flexBasis: '47%', flexGrow: 1, borderWidth: StyleSheet.hairlineWidth, borderRadius: 16, padding: 14, gap: 2 },
});

import { Image } from 'expo-image';
import * as ImagePicker from 'expo-image-picker';
import { useLocalSearchParams } from 'expo-router';
import { useState } from 'react';
import { Linking, RefreshControl, ScrollView, StyleSheet, View } from 'react-native';
import { SafeAreaView } from 'react-native-safe-area-context';

import { Button, Card, Chip, Notice, Row, Text } from '@/components/ui';
import { errorMessage } from '@/core/api/envelope';
import { useAuth } from '@/core/auth/auth-provider';
import { useColors } from '@/core/theme';
import { statusLabel } from '@/features/receipts/fields';
import { useReceipts } from '@/features/receipts/provider';
import { taskItems } from '@/features/receipts/queue';
import { statusTone } from '@/features/receipts/status';
import { taskStatusLabel, transactionLine } from '@/features/tasks/tasks';
import { useTask } from '@/features/tasks/use-tasks';

const dayLabel = (iso: string) =>
  new Intl.DateTimeFormat('tr-TR', { day: 'numeric', month: 'long', year: 'numeric' }).format(new Date(iso));

/** Görev ayrıntısı (#200): ofisin istediği belge, banka hareketi, yüklenenler ve fotoğrafla yükleme. */
export default function TaskScreen() {
  const { uuid } = useLocalSearchParams<{ uuid: string }>();
  const task = useTask(uuid ?? '');
  const { items, enqueueTask, online, retry } = useReceipts();
  const { taxpayer } = useAuth();
  const c = useColors();
  const [denied, setDenied] = useState(false);
  const [saving, setSaving] = useState(false);
  const queued = taskItems(items, taxpayer?.taxpayer.uuid ?? '', uuid ?? '').filter((i) => i.status !== 'uploaded');
  const t = task.data;
  const closed = t ? t.status === 'matched' || t.status === 'cancelled' : false;

  async function add(fromCamera: boolean) {
    if (fromCamera) {
      const perm = await ImagePicker.requestCameraPermissionsAsync();
      if (!perm.granted) {
        setDenied(true);
        return;
      }
    }
    const res = fromCamera
      ? await ImagePicker.launchCameraAsync({ mediaTypes: ['images'], allowsEditing: true, quality: 0.7 })
      : await ImagePicker.launchImageLibraryAsync({ mediaTypes: ['images'], allowsMultipleSelection: true, selectionLimit: 10, quality: 0.7 });
    if (res.canceled || !uuid) return;
    setSaving(true);
    try {
      await enqueueTask(
        uuid,
        res.assets.map((a) => ({ uri: a.uri, mimeType: a.mimeType })),
      );
    } finally {
      setSaving(false);
    }
  }

  return (
    <SafeAreaView style={{ flex: 1, backgroundColor: c.background }} edges={['left', 'right', 'bottom']}>
      <ScrollView
        contentContainerStyle={{ padding: 20, gap: 12, maxWidth: 640, width: '100%', alignSelf: 'center' }}
        refreshControl={<RefreshControl refreshing={task.isFetching} onRefresh={() => void task.refetch()} />}>
        {task.isError ? <Notice>{errorMessage(task.error)}</Notice> : null}
        {t ? (
          <>
            <Card>
              <View style={styles.head}>
                <Text variant="heading" style={{ flex: 1 }}>
                  {t.kind === 'undocumented_payment' ? 'Ödeme için belge' : 'Tahsilat için fatura'}
                </Text>
                <Chip label={t.overdue && !closed ? 'Gecikti' : taskStatusLabel(t.status)} tone={t.overdue && !closed ? 'danger' : closed ? 'success' : 'brand'} />
              </View>
              <Text>{transactionLine(t)}</Text>
              {t.bank_transaction.description ? <Text tone="muted">{t.bank_transaction.description}</Text> : null}
              {t.note ? <Text>Müşavirin notu: {t.note}</Text> : null}
              <Text variant="caption" tone="muted">
                {t.organization_name ? `${t.organization_name} · ` : ''}Son gün {dayLabel(t.due_at)}
              </Text>
            </Card>

            {closed ? (
              <Notice tone="info">{t.status === 'matched' ? 'Belge alındı ve hareketle eşleşti. Teşekkürler.' : 'Müşaviriniz bu isteği kapattı.'}</Notice>
            ) : (
              <>
                {denied ? (
                  <Notice>
                    Kamera izni verilmedi.{' '}
                    <Text tone="brand" onPress={() => Linking.openSettings()}>
                      Ayarları aç
                    </Text>
                  </Notice>
                ) : null}
                <Button title="Belgenin fotoğrafını çek" onPress={() => void add(true)} loading={saving} />
                <Button title="Galeriden seç" variant="secondary" onPress={() => void add(false)} disabled={saving} />
                {!online ? (
                  <Text variant="caption" tone="muted">
                    Bağlantı yok: fotoğraflar telefonda saklanır, bağlantı gelince gönderilir.
                  </Text>
                ) : null}
              </>
            )}

            {queued.length ? (
              <View style={{ gap: 8 }}>
                <Text variant="heading">Telefonda bekleyenler</Text>
                {queued.map((i) => (
                  <Row
                    key={i.id}
                    title="Fotoğraf"
                    subtitle={i.lastError ?? undefined}
                    onPress={i.status === 'failed' ? () => retry(i.id) : undefined}
                    leading={
                      <View style={[styles.thumb, { borderColor: c.border }]}>
                        <Image source={{ uri: i.fileUri }} style={StyleSheet.absoluteFill} contentFit="cover" />
                      </View>
                    }
                    trailing={<Chip label={statusLabel(i)} tone={statusTone(i)} />}
                  />
                ))}
              </View>
            ) : null}

            {t.uploads?.length ? (
              <View style={{ gap: 8 }}>
                <Text variant="heading">Gönderilenler</Text>
                {t.uploads.map((u) => (
                  <Row key={u.uuid} title={u.file_name ?? (u.kind === 'pdf' ? 'PDF' : 'Fotoğraf')} subtitle={dayLabel(u.created_at)} />
                ))}
              </View>
            ) : null}
          </>
        ) : null}
      </ScrollView>
    </SafeAreaView>
  );
}

const styles = StyleSheet.create({
  head: { flexDirection: 'row', alignItems: 'center', gap: 8 },
  thumb: { width: 44, height: 58, borderRadius: 8, overflow: 'hidden', borderWidth: StyleSheet.hairlineWidth },
});

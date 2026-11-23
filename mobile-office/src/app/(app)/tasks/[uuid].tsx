import { Image } from 'expo-image';
import { useLocalSearchParams } from 'expo-router';
import { useState } from 'react';
import { ActivityIndicator, StyleSheet, View } from 'react-native';

import { Button, Card, Chip, Field, Notice, Screen, Text } from '@/components/ui';
import { errorMessage } from '@/core/api/envelope';
import { useAuth, useOrganization } from '@/core/auth/auth-provider';
import { useColors } from '@/core/theme';
import { money } from '@/features/clients/format';
import { uploadFileSource, useTask, useTaskAction } from '@/features/doctasks/api';
import { canRemind } from '@/features/doctasks/channels';
import { RemindButton } from '@/features/doctasks/remind-button';
import { actions, dayLabel, statusLabel, statusTone, taskTitle, type TaskStatus } from '@/features/doctasks/format';

/** Görev ayrıntısı (#240): durum, gelen yüklemeler; tamamlandı / yeniden iste / iptal. */
export default function TaskScreen() {
  const { uuid, engagement } = useLocalSearchParams<{ uuid: string; engagement: string }>();
  const org = useOrganization();
  const { can } = useAuth();
  const c = useColors();
  const q = useTask(engagement, uuid);
  const act = useTaskAction(engagement, uuid);
  const [asking, setAsking] = useState(false);
  const [note, setNote] = useState('');
  const t = q.data;
  const status = (t?.status ?? 'open') as TaskStatus;
  const allowed = can('tenant.doc_tasks.write');
  const a = actions(status);
  const bankTask = t ? t.kind !== 'document_request' : false;

  return (
    <Screen>
      {q.isLoading ? <ActivityIndicator color={c.brand} /> : null}
      {q.error ? <Notice>{errorMessage(q.error)}</Notice> : null}
      {t ? (
        <>
          <View style={{ flexDirection: 'row', alignItems: 'center', gap: 8 }}>
            <Text variant="title" style={{ flex: 1 }} numberOfLines={3}>
              {taskTitle(t)}
            </Text>
            <Chip tone={t.overdue ? 'danger' : statusTone(status)} label={t.overdue ? 'Gecikti' : statusLabel[status] ?? t.status} />
          </View>
          <Card>
            {t.note ? <Text>{t.note}</Text> : null}
            {bankTask ? (
              <Text tone="muted">
                {money(t.bank_transaction.amount, t.bank_transaction.currency)} · {t.bank_transaction.counterparty_name || t.bank_transaction.description} ·{' '}
                {t.bank_transaction.transaction_date}
              </Text>
            ) : null}
            <Text variant="caption" tone="muted">
              Son gün {dayLabel(t.due_at)}
              {t.request_count && t.request_count > 1 ? ` · ${t.request_count}. istek` : ''}
              {typeof t.reminder_count === 'number' && t.reminder_count > 0 ? ` · ${t.reminder_count} hatırlatma` : ''}
            </Text>
          </Card>

          <Text variant="heading">Gelen dosyalar ({t.uploads?.length ?? 0})</Text>
          {t.uploads?.length ? (
            <View style={styles.thumbs}>
              {t.uploads.map((u) =>
                u.content_type?.startsWith('image/') ? (
                  <Image
                    key={u.uuid}
                    source={uploadFileSource(org, engagement, t.uuid, u.uuid)}
                    style={[styles.thumb, { backgroundColor: c.border }]}
                    contentFit="cover"
                    accessibilityLabel={u.file_name ?? 'Yüklenen görsel'}
                  />
                ) : (
                  <View key={u.uuid} style={[styles.thumb, styles.file, { backgroundColor: c.border }]}>
                    <Text variant="caption">PDF</Text>
                    <Text variant="caption" numberOfLines={2}>
                      {u.file_name}
                    </Text>
                  </View>
                ),
              )}
            </View>
          ) : (
            <Text tone="muted">Mükellef henüz dosya yüklemedi.</Text>
          )}

          {act.error ? <Notice>{errorMessage(act.error)}</Notice> : null}

          {allowed && asking ? (
            <Card>
              <Field label="Ne eksik?" value={note} onChangeText={setNote} multiline style={{ minHeight: 72 }} maxLength={1000} />
              <Button
                title="Yeniden iste (7 gün)"
                onPress={async () => {
                  try {
                    await act.mutateAsync({ kind: 'rerequest', note: note.trim(), due_days: 7 });
                    setAsking(false);
                    setNote('');
                  } catch {
                    // act.error
                  }
                }}
                disabled={note.trim().length < 3}
                loading={act.isPending}
              />
              <Button title="Vazgeç" variant="ghost" onPress={() => setAsking(false)} />
            </Card>
          ) : null}

          {allowed && !asking ? (
            <View style={{ gap: 10 }}>
              {canRemind(status) ? <RemindButton engagement={engagement} task={t.uuid} title={taskTitle(t)} /> : null}
              {a.complete ? <Button title="Tamamlandı" onPress={() => void act.mutateAsync({ kind: 'complete' }).catch(() => {})} loading={act.isPending} /> : null}
              {a.rerequest ? <Button title="Yeniden iste" variant="secondary" onPress={() => setAsking(true)} disabled={act.isPending} /> : null}
              {a.cancel ? <Button title="İptal et" variant="ghost" onPress={() => void act.mutateAsync({ kind: 'cancel' }).catch(() => {})} disabled={act.isPending} /> : null}
            </View>
          ) : null}
        </>
      ) : null}
    </Screen>
  );
}

const styles = StyleSheet.create({
  thumbs: { flexDirection: 'row', flexWrap: 'wrap', gap: 10 },
  thumb: { width: 104, height: 140, borderRadius: 10 },
  file: { alignItems: 'center', justifyContent: 'center', padding: 8, gap: 4 },
});

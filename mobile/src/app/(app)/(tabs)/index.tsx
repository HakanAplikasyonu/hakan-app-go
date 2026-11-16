import { useCallback, useEffect, useState } from 'react';
import { Pressable, StyleSheet, View } from 'react-native';

import { router } from 'expo-router';

import { Button, Card, Chip, Row, Screen, Text } from '@/components/ui';
import { useAuth } from '@/core/auth/auth-provider';
import { initials, kindLabel, roleLabel } from '@/core/auth/taxpayers';
import { enablePush, getPushPermission, type PushPermission } from '@/core/push/push';
import { useColors } from '@/core/theme';
import { receiptsSummaryLine, serverOnly } from '@/features/receipts/history';
import { useReceipts } from '@/features/receipts/provider';
import { QueueBanner } from '@/features/receipts/queue-banner';
import { useReceiptHistory } from '@/features/receipts/use-history';
import { openTask, useTasks } from '@/features/tasks/use-tasks';

/** Mükellef ana sayfası: fiş çek, görevler, fişlerim. */
export default function HomeScreen() {
  const { me, taxpayer, taxpayers, switchTaxpayer, signOut } = useAuth();
  const c = useColors();
  const [push, setPush] = useState<PushPermission>('unavailable');
  const [pushBusy, setPushBusy] = useState(false);
  const { summary, items } = useReceipts();
  const { history } = useReceiptHistory();
  const { tasks } = useTasks();

  useEffect(() => {
    getPushPermission().then(setPush);
  }, []);

  const turnOnPush = useCallback(async () => {
    setPushBusy(true);
    try {
      await enablePush();
      setPush(await getPushPermission());
    } finally {
      setPushBusy(false);
    }
  }, []);

  if (!taxpayer) return null;
  const t = taxpayer.taxpayer;

  return (
    <Screen style={{ maxWidth: 640, width: '100%', alignSelf: 'center' }}>
      <Pressable
        accessibilityRole="button"
        accessibilityLabel={`Şirket: ${t.title}. Değiştir`}
        onPress={switchTaxpayer}
        disabled={taxpayers.length < 2}
        style={[styles.company, { backgroundColor: c.surface, borderColor: c.border }]}>
        <View style={[styles.avatar, { backgroundColor: c.primary }]}>
          <Text style={{ color: c.primaryText, fontWeight: '700', fontSize: 18 }}>{initials(t.title)}</Text>
        </View>
        <View style={{ flex: 1, gap: 2 }}>
          <Text variant="heading" numberOfLines={2}>
            {t.title}
          </Text>
          <Text variant="caption" tone="muted" style={{ fontVariant: ['tabular-nums'] }}>
            {t.tax_id_type.toUpperCase()} {t.tax_id} · {kindLabel(t.kind)} · {roleLabel(taxpayer.role)}
          </Text>
        </View>
        {taxpayers.length > 1 ? (
          <Text variant="caption" tone="brand">
            Değiştir
          </Text>
        ) : null}
      </Pressable>

      <View style={{ gap: 4 }}>
        <Text variant="title">Merhaba{me?.user.name ? `, ${me.user.name}` : ''}</Text>
        <Text tone="muted">Fişlerinizi ve müşavirinizin istediği belgeleri buradan göndereceksiniz.</Text>
      </View>

      <QueueBanner />
      <Button title="Fiş / Z raporu çek" onPress={() => router.push('/capture')} />

      <Card>
        <View style={{ flexDirection: 'row', alignItems: 'center', justifyContent: 'space-between' }}>
          <Text variant="heading">Görevler</Text>
          {tasks.length ? <Chip label={String(tasks.length)} tone="warning" /> : null}
        </View>
        {tasks.length === 0 ? <Text tone="muted">Şu an yapmanız gereken bir şey yok.</Text> : null}
        {tasks.slice(0, 3).map((t) => (
          <Row key={t.id} title={t.title} subtitle={t.detail || undefined} onPress={() => openTask(t)} />
        ))}
        <Button title="Tüm görevler" variant="ghost" onPress={() => router.push('/tasks')} />
      </Card>

      <Row
        title="Fişlerim"
        subtitle={receiptsSummaryLine(summary, serverOnly(history, items, t.uuid).length)}
        onPress={() => router.push('/receipts')}
        trailing={<Text tone="muted">›</Text>}
      />

      {push !== 'granted' ? (
        <Card>
          <Text variant="heading">Bildirimler</Text>
          <Text tone="muted">
            {push === 'unavailable'
              ? 'Bu cihazda anlık bildirim kullanılamıyor.'
              : push === 'denied'
                ? 'Bildirimler kapalı. Telefon ayarlarından açabilirsiniz.'
                : 'Müşaviriniz belge istediğinde veya bir kayıt onayınızı beklediğinde haber verelim.'}
          </Text>
          {push === 'undetermined' ? <Button title="Bildirimleri aç" onPress={turnOnPush} loading={pushBusy} /> : null}
        </Card>
      ) : null}

      <View style={{ flex: 1 }} />
      <Text variant="caption" tone="muted" style={{ textAlign: 'center' }}>
        {me?.user.email}
      </Text>
      <Button title="Profil ve hesap" variant="secondary" onPress={() => router.push('/profile')} />
      <Button title="Çıkış yap" variant="ghost" onPress={signOut} />
    </Screen>
  );
}

const styles = StyleSheet.create({
  company: { flexDirection: 'row', alignItems: 'center', gap: 14, borderWidth: StyleSheet.hairlineWidth, borderRadius: 16, padding: 14 },
  avatar: { width: 48, height: 48, borderRadius: 14, alignItems: 'center', justifyContent: 'center' },
});

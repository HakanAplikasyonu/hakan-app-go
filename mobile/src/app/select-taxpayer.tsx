import { useMemo, useState } from 'react';
import { ActivityIndicator, Pressable, StyleSheet, View } from 'react-native';

import { BrandMark } from '@/components/brand-mark';
import { Button, Card, Notice, Screen, Text } from '@/components/ui';
import { errorMessage } from '@/core/api/envelope';
import { useAuth } from '@/core/auth/auth-provider';
import { initials, kindLabel, roleLabel, sortTaxpayers } from '@/core/auth/taxpayers';
import { useColors } from '@/core/theme';

/** Şirket (mükellef) seçici. Üyeliği olmayan kullanıcı da buraya düşer. */
export default function SelectTaxpayerScreen() {
  const { me, taxpayers, taxpayer: current, selectTaxpayer, reload, signOut } = useAuth();
  const c = useColors();
  const [pending, setPending] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [refreshing, setRefreshing] = useState(false);
  const list = useMemo(() => sortTaxpayers(taxpayers), [taxpayers]);

  async function choose(uuid: string) {
    setPending(uuid);
    setError(null);
    try {
      await selectTaxpayer(uuid);
    } catch (e) {
      setError(errorMessage(e));
    } finally {
      setPending(null);
    }
  }

  async function refresh() {
    setRefreshing(true);
    setError(null);
    try {
      await reload();
    } catch (e) {
      setError(errorMessage(e));
    } finally {
      setRefreshing(false);
    }
  }

  return (
    <Screen style={{ maxWidth: 560, width: '100%', alignSelf: 'center' }}>
      <View style={{ gap: 8 }}>
        <BrandMark size={44} />
        <Text variant="title">Şirket seçin</Text>
        <Text tone="muted">
          {me?.user.name ? `Merhaba ${me.user.name}. ` : ''}
          {list.length ? 'Hangi şirketin belgeleriyle çalışacaksınız?' : ''}
        </Text>
      </View>

      {error ? <Notice>{error}</Notice> : null}

      {list.length === 0 ? (
        <Card>
          <Text variant="heading">Henüz bir şirkete bağlı değilsiniz</Text>
          <Text tone="muted">
            Mali müşavirinizin gönderdiği daveti e-postanızdan kabul edin. Ardından bu ekranı yenileyin.
          </Text>
          <Button title="Yenile" variant="secondary" onPress={refresh} loading={refreshing} />
        </Card>
      ) : (
        <View style={{ gap: 10 }} accessibilityRole="list">
          {list.map((t) => {
            const active = current?.taxpayer.uuid === t.uuid;
            return (
              <Pressable
                key={t.uuid}
                accessibilityRole="button"
                accessibilityLabel={`${t.title}, ${kindLabel(t.kind)}, ${roleLabel(t.role)}`}
                disabled={pending !== null}
                onPress={() => choose(t.uuid)}
                style={({ pressed }) => [
                  styles.row,
                  { backgroundColor: c.surface, borderColor: active ? c.brand : c.border, opacity: pressed ? 0.85 : 1 },
                ]}>
                <View style={[styles.avatar, { backgroundColor: c.primary }]}>
                  <Text style={{ color: c.primaryText, fontWeight: '700' }}>{initials(t.title)}</Text>
                </View>
                <View style={{ flex: 1, gap: 2 }}>
                  <Text variant="heading" numberOfLines={2}>
                    {t.title}
                  </Text>
                  <Text variant="caption" tone="muted">
                    {kindLabel(t.kind)} · {roleLabel(t.role)}
                    {active ? ' · Açık' : ''}
                  </Text>
                </View>
                {pending === t.uuid ? <ActivityIndicator color={c.brand} /> : <Text tone="muted">›</Text>}
              </Pressable>
            );
          })}
        </View>
      )}

      <View style={{ flex: 1 }} />
      <Button title="Çıkış yap" variant="ghost" onPress={signOut} />
    </Screen>
  );
}

const styles = StyleSheet.create({
  row: { flexDirection: 'row', alignItems: 'center', gap: 14, borderWidth: 1, borderRadius: 16, padding: 14 },
  avatar: { width: 44, height: 44, borderRadius: 12, alignItems: 'center', justifyContent: 'center' },
});

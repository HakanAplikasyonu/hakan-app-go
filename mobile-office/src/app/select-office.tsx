// Yapı: mobile/src/app/select-taxpayer.tsx (#236); ofis üyelikleri için.
import { useMemo, useState } from 'react';
import { ActivityIndicator, Pressable, StyleSheet, View } from 'react-native';

import { BrandMark } from '@/components/brand-mark';
import { Button, Notice, Screen, Text } from '@/components/ui';
import { errorMessage } from '@/core/api/envelope';
import { useAuth } from '@/core/auth/auth-provider';
import { initials, isSelectable, sortOffices, statusLabel } from '@/core/auth/offices';
import { useColors } from '@/core/theme';

export default function SelectOfficeScreen() {
  const { me, offices, officeSlug, selectOffice, signOut } = useAuth();
  const c = useColors();
  const [pending, setPending] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);
  const list = useMemo(() => sortOffices(offices), [offices]);

  async function choose(slug: string) {
    setPending(slug);
    setError(null);
    try {
      await selectOffice(slug);
    } catch (e) {
      setError(errorMessage(e));
    } finally {
      setPending(null);
    }
  }

  return (
    <Screen style={{ maxWidth: 560, width: '100%', alignSelf: 'center' }}>
      <View style={{ gap: 8 }}>
        <BrandMark size={44} />
        <Text variant="title">Ofis seçin</Text>
        <Text tone="muted">
          {me?.user.name ? `Merhaba ${me.user.name}. ` : ''}Hangi ofisin işleriyle çalışacaksınız?
        </Text>
      </View>

      {error ? <Notice>{error}</Notice> : null}

      <View style={{ gap: 10 }} accessibilityRole="list">
        {list.map((o) => {
          const active = officeSlug === o.slug;
          const open = isSelectable(o);
          const status = statusLabel(o);
          return (
            <Pressable
              key={o.uuid}
              testID={`office-${o.slug}`}
              accessibilityRole="button"
              accessibilityState={{ disabled: !open }}
              accessibilityLabel={`${o.name}, ${o.role.name}${status ? `, ${status}` : ''}`}
              disabled={pending !== null || !open}
              onPress={() => choose(o.slug)}
              style={({ pressed }) => [
                styles.row,
                { backgroundColor: c.surface, borderColor: active ? c.brand : c.border, opacity: !open ? 0.5 : pressed ? 0.85 : 1 },
              ]}>
              <View style={[styles.avatar, { backgroundColor: c.primary }]}>
                <Text style={{ color: c.primaryText, fontWeight: '700' }}>{initials(o.name)}</Text>
              </View>
              <View style={{ flex: 1, gap: 2 }}>
                <Text variant="heading" numberOfLines={2}>
                  {o.name}
                </Text>
                <Text variant="caption" tone={status ? 'danger' : 'muted'}>
                  {o.role.name}
                  {status ? ` · ${status}` : active ? ' · Açık' : ''}
                </Text>
              </View>
              {pending === o.slug ? <ActivityIndicator color={c.brand} /> : <Text tone="muted">›</Text>}
            </Pressable>
          );
        })}
      </View>

      <View style={{ flex: 1 }} />
      <Button title="Çıkış yap" variant="ghost" onPress={signOut} />
    </Screen>
  );
}

const styles = StyleSheet.create({
  row: { flexDirection: 'row', alignItems: 'center', gap: 14, borderWidth: 1, borderRadius: 16, padding: 14 },
  avatar: { width: 44, height: 44, borderRadius: 12, alignItems: 'center', justifyContent: 'center' },
});

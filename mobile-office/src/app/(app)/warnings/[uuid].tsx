import { useLocalSearchParams, useRouter } from 'expo-router';
import { useState } from 'react';
import { View } from 'react-native';

import { Button, Card, Chip, Notice, Screen, Text } from '@/components/ui';
import { errorMessage } from '@/core/api/envelope';
import { useAuth } from '@/core/auth/auth-provider';
import { useCachedWarning, useDecideWarning } from '@/features/warnings/api';
import { canDecide, periodLabel, severityLabel, severityTone, statusLabel, warningMessage } from '@/features/warnings/format';

/** Uyarı ayrıntısı: Gördüm / Çözüldü. Karar ofis panelinde de aynı satırda görünür. */
export default function WarningDetailScreen() {
  const { uuid } = useLocalSearchParams<{ uuid: string }>();
  const router = useRouter();
  const { can } = useAuth();
  const warning = useCachedWarning(uuid);
  const decide = useDecideWarning();
  const [done, setDone] = useState<string | null>(null);

  if (!warning) {
    return (
      <Screen>
        <Notice tone="info">Uyarı bulunamadı. Listeyi yenileyip tekrar deneyin.</Notice>
        <Button title="Uyarılara dön" variant="secondary" onPress={() => router.back()} />
      </Screen>
    );
  }

  const allowed = can('tenant.warnings.write');
  const open = canDecide(warning);

  async function run(decision: 'acknowledge' | 'resolve') {
    if (!warning) return;
    setDone(null);
    try {
      const res = await decide.mutateAsync({ warning, decision });
      setDone(res.status === 'resolved' ? 'Uyarı çözüldü olarak işaretlendi.' : 'Uyarı görüldü olarak işaretlendi.');
    } catch {
      // decide.error gösterilir
    }
  }

  return (
    <Screen>
      <View style={{ flexDirection: 'row', gap: 8, flexWrap: 'wrap' }}>
        <Chip tone={severityTone(warning.severity)} label={severityLabel[warning.severity]} />
        <Chip label={statusLabel[warning.status]} />
        {warning.period ? <Chip label={periodLabel(warning.period) ?? warning.period} /> : null}
      </View>
      <Text variant="heading">{warningMessage(warning.code, warning.params)}</Text>
      <Card>
        <Text variant="caption" tone="muted">
          Kod
        </Text>
        <Text selectable>{warning.code}</Text>
        <Text variant="caption" tone="muted">
          Kaynak
        </Text>
        <Text>{warning.producer}</Text>
        <Text variant="caption" tone="muted">
          Oluştu
        </Text>
        <Text>{new Date(warning.created_at).toLocaleString('tr-TR', { timeZone: 'Europe/Istanbul' })}</Text>
      </Card>

      {done ? <Notice tone="info">{done}</Notice> : null}
      {decide.error ? <Notice>{errorMessage(decide.error)}</Notice> : null}

      {open && allowed ? (
        <View style={{ gap: 10 }}>
          <Button title="Çözüldü" onPress={() => run('resolve')} loading={decide.isPending && decide.variables?.decision === 'resolve'} disabled={decide.isPending} />
          <Button
            title="Gördüm"
            variant="secondary"
            onPress={() => run('acknowledge')}
            loading={decide.isPending && decide.variables?.decision === 'acknowledge'}
            disabled={decide.isPending}
          />
        </View>
      ) : null}
      {open && !allowed ? (
        <Text variant="caption" tone="muted">
          Uyarılar hakkında karar verme yetkiniz yok.
        </Text>
      ) : null}
    </Screen>
  );
}

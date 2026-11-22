import { HataDurumu, KristalYukleyici } from '@hakan/mobile-core/hakan';
import { useRouter } from 'expo-router';
import { StyleSheet, View } from 'react-native';

import { Button, Card, Chip, Text } from '@/components/ui';
import { errorMessage } from '@/core/api/envelope';
import { useColors } from '@/core/theme';
import { periodLabel } from '@/features/warnings/format';

import { useEngagementWorkflow, type WorkflowRow } from './api';
import { actionLabels, idleLabel, mobileTarget, nextActionLabel, nextStepOf, progressLabel, statusLabels, stepDetail, stepLabels } from './format';

/**
 * Mükellef ayrıntısında "Sıradaki iş" kartı (#392, mobilde #454): dönem
 * kapanış zinciri ve sıradaki adım. Ekran açan adımlar uygulamadaki ekrana
 * gider; onay isteyen adımlar (banka, fiş, aktarım, rapor) ofis panelinde
 * çalışır, burada yalnızca görünür.
 */
export function NextWorkCard({ engagement, title }: { engagement: string; title: string }) {
  const q = useEngagementWorkflow(engagement);

  return (
    <Card>
      <View testID="copilot-card" style={styles.head}>
        <Text variant="heading" accessibilityRole="header" style={{ flex: 1 }}>
          Sıradaki iş{q.data ? ` · ${periodLabel(q.data.period)}` : ''}
        </Text>
        {q.data ? (
          <Text variant="caption" tone="muted">
            {progressLabel(q.data)}
          </Text>
        ) : null}
      </View>
      {q.isLoading ? (
        <View style={styles.center}>
          <KristalYukleyici etiket="Sıradaki iş yükleniyor" />
        </View>
      ) : q.error ? (
        <HataDurumu
          baslik="Sıradaki iş alınamadı"
          aciklama={errorMessage(q.error)}
          style={{ paddingVertical: 12 }}
          eylem={<Button testID="copilot-retry" title="Tekrar dene" variant="secondary" loading={q.isRefetching} onPress={() => void q.refetch()} />}
        />
      ) : q.data ? (
        <WorkflowBody row={q.data} engagement={engagement} title={title} />
      ) : null}
    </Card>
  );
}

function WorkflowBody({ row, engagement, title }: { row: WorkflowRow; engagement: string; title: string }) {
  const router = useRouter();
  const step = nextStepOf(row);
  const target = mobileTarget(step?.action);

  return (
    <View style={{ gap: 10 }}>
      {step ? (
        <View testID="copilot-next" style={{ gap: 6 }} accessible accessibilityLabel={`Sıradaki iş: ${stepLabels[step.key]}. ${stepDetail(step.key, step.status, step.count)}`}>
          <Text style={{ fontWeight: '600' }}>
            {stepLabels[step.key]}: {stepDetail(step.key, step.status, step.count)}
          </Text>
          {step.action && target === 'web' ? (
            <Text variant="caption" tone="muted">
              {nextActionLabel(step.action)}: ofis panelinde yapılır.
            </Text>
          ) : null}
        </View>
      ) : (
        <Text testID="copilot-idle" tone="muted">
          {idleLabel(row)}
        </Text>
      )}
      {step?.action && target === 'review' ? (
        <Button testID="copilot-action" title={actionLabels[step.action]} onPress={() => router.push('/review')} />
      ) : null}
      {step?.action && target === 'tasks' ? (
        <Button
          testID="copilot-action"
          title={actionLabels[step.action]}
          onPress={() => router.push({ pathname: '/tasks', params: { engagement, title } })}
        />
      ) : null}
      <ChainStrip row={row} />
      <View accessibilityLabel="Dönem kapanış adımları" style={{ gap: 6 }}>
        {row.steps.map((s) => (
          <View key={s.key} testID={`copilot-step-${s.key}`} style={styles.step} accessible accessibilityLabel={`${stepLabels[s.key]}: ${statusLabels[s.status].label}. ${stepDetail(s.key, s.status, s.count)}`}>
            <View style={{ flex: 1, gap: 2 }}>
              <Text>{stepLabels[s.key]}</Text>
              {s.status !== 'na' ? (
                <Text variant="caption" tone="muted">
                  {stepDetail(s.key, s.status, s.count)}
                </Text>
              ) : null}
            </View>
            <Chip tone={statusLabels[s.status].tone} label={statusLabels[s.status].label} />
          </View>
        ))}
      </View>
    </View>
  );
}

/**
 * Ay sonu zinciri şeridi (#802): her adım bir renkli dilim (tamam yeşil,
 * yapılacak marka rengi, bekleniyor sarı, sorun kırmızı, bu dönem yok gri).
 * Yalnız gösterim; adımların ayrıntısı altındaki listede.
 */
function ChainStrip({ row }: { row: WorkflowRow }) {
  const c = useColors();
  const color = { neutral: c.muted, brand: c.brand, success: c.success, warning: c.warning, danger: c.danger };
  const summary = row.steps.map((s) => `${stepLabels[s.key]}: ${statusLabels[s.status].label}`).join(', ');
  return (
    <View testID="copilot-chain-strip" style={styles.strip} accessible accessibilityLabel={`Ay sonu zinciri. ${summary}`}>
      {row.steps.map((s) => (
        <View
          key={s.key}
          testID={`copilot-strip-${s.key}-${s.status}`}
          style={[styles.segment, { backgroundColor: color[statusLabels[s.status].tone], opacity: s.status === 'na' ? 0.35 : 1 }]}
        />
      ))}
    </View>
  );
}

const styles = StyleSheet.create({
  strip: { flexDirection: 'row', gap: 3 },
  segment: { flex: 1, height: 6, borderRadius: 3 },
  head: { flexDirection: 'row', alignItems: 'center', gap: 8 },
  center: { alignItems: 'center', paddingVertical: 16 },
  step: { flexDirection: 'row', alignItems: 'center', gap: 8, minHeight: 36 },
});

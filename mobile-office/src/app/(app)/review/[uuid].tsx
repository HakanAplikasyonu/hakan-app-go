import { Image } from 'expo-image';
import { useLocalSearchParams, useRouter } from 'expo-router';
import { useState } from 'react';
import { ActivityIndicator, View } from 'react-native';

import { Button, Card, Chip, Notice, Screen, Text } from '@/components/ui';
import { errorMessage } from '@/core/api/envelope';
import { useAuth, useOrganization } from '@/core/auth/auth-provider';
import { useColors } from '@/core/theme';
import { receiptImageSource } from '@/features/clients/api';
import { money } from '@/features/clients/format';
import { useDecide, useSuggestion, useSuggestionContext } from '@/features/review/api';
import { confidenceBand, confidencePercent, fieldLabel, proposedCode, proposedText } from '@/features/review/format';
import { ReasonSheet, type ReasonSheetMode } from '@/features/review/reason-sheet';
import { periodLabel } from '@/features/warnings/format';

/** Öneri ayrıntısı (#239): belge, önerilen hesap kodu ve gerekçe; onayla / düzelt / reddet. */
export default function SuggestionScreen() {
  const { uuid, engagement } = useLocalSearchParams<{ uuid: string; engagement: string }>();
  const router = useRouter();
  const org = useOrganization();
  const { can } = useAuth();
  const c = useColors();
  const s = useSuggestion(engagement, uuid);
  const ctx = useSuggestionContext(engagement, uuid);
  const d = useDecide();
  const [sheet, setSheet] = useState<ReasonSheetMode | null>(null);
  const [done, setDone] = useState<string | null>(null);

  const sug = s.data;
  const code = sug ? proposedCode(sug.proposed_value) : null;
  const pending = sug?.status === 'pending';
  const allowed = can('tenant.review.decide');
  const doc = ctx.data;

  async function run(decision: Parameters<typeof d.mutateAsync>[0]['decision'], message: string) {
    if (!sug) return;
    try {
      await d.mutateAsync({ suggestion: { uuid: sug.uuid, engagement_uuid: engagement }, decision });
      setSheet(null);
      setDone(message);
      void s.refetch();
    } catch {
      // d.error gösterilir
    }
  }

  return (
    <Screen>
      {s.isLoading ? <ActivityIndicator color={c.brand} /> : null}
      {s.error ? <Notice>{errorMessage(s.error)}</Notice> : null}

      {doc?.receipt && doc.receipt.media_type.startsWith('image/') ? (
        <Image
          source={receiptImageSource(org, engagement, doc.receipt.uuid)}
          style={{ width: '100%', aspectRatio: 3 / 4, borderRadius: 16, backgroundColor: c.border }}
          contentFit="contain"
          accessibilityLabel="Fiş görseli"
        />
      ) : null}

      {doc?.invoice ? (
        <Card>
          <Text variant="caption" tone="muted">
            {doc.invoice.direction === 'incoming' ? 'Gelen fatura' : 'Giden fatura'} · {doc.invoice.invoice_number}
          </Text>
          <Text variant="heading">{doc.invoice.counterparty_name || '—'}</Text>
          <Text tone="muted">
            {doc.invoice.issue_date} · {money(doc.invoice.payable_amount, doc.invoice.currency)}
          </Text>
          {doc.item ? (
            <View style={{ gap: 2, marginTop: 6 }}>
              <Text style={{ fontWeight: '600' }}>
                {doc.item.line_no}. kalem: {doc.item.name || '—'}
              </Text>
              <Text variant="caption" tone="muted">
                {doc.item.quantity} {doc.item.unit_code ?? ''} · {money(doc.item.line_amount, doc.invoice.currency)} · KDV %{Number(doc.item.tax_rate)}
              </Text>
            </View>
          ) : null}
        </Card>
      ) : ctx.isLoading ? (
        <ActivityIndicator color={c.brand} />
      ) : null}

      {sug ? (
        <Card>
          <View style={{ flexDirection: 'row', alignItems: 'center', gap: 8 }}>
            <Text variant="caption" tone="muted" style={{ flex: 1 }}>
              Önerilen {fieldLabel(sug.target_field).toLocaleLowerCase('tr')}
              {sug.period ? ` · ${periodLabel(sug.period)}` : ''}
            </Text>
            <Chip
              tone={confidenceBand(sug.confidence) === 'high' ? 'success' : confidenceBand(sug.confidence) === 'medium' ? 'warning' : 'danger'}
              label={`Güven ${confidencePercent(sug.confidence)}`}
            />
          </View>
          <Text variant="title">{sug ? proposedText(sug.proposed_value) || '—' : '—'}</Text>
          <Text variant="caption" tone="muted">
            Gerekçe
          </Text>
          <Text>{sug.rationale}</Text>
          <Text variant="caption" tone="muted">
            Kaynak: {sug.provenance.origin === 'rule' ? `kural ${sug.provenance.rule_ref ?? ''}` : sug.provenance.origin === 'model' ? `yapay zekâ ${sug.provenance.model ?? ''}` : sug.provenance.origin}
          </Text>
          {!pending ? <Chip label={`Durum: ${sug.status}`} /> : null}
        </Card>
      ) : null}

      {done ? <Notice tone="info">{done}</Notice> : null}
      {d.error && !sheet ? <Notice>{errorMessage(d.error)}</Notice> : null}

      {sug && pending && allowed ? (
        <View style={{ gap: 10 }}>
          <Button title="Onayla" onPress={() => void run({ action: 'accept' }, `${code ?? 'Öneri'} onaylandı.`)} loading={d.isPending && !sheet} disabled={d.isPending} />
          <Button title="Düzelt" variant="secondary" onPress={() => setSheet({ kind: 'correct', initialCode: code ?? '' })} disabled={d.isPending} />
          <Button title="Reddet" variant="ghost" onPress={() => setSheet({ kind: 'reject' })} disabled={d.isPending} />
        </View>
      ) : null}
      {sug && pending && !allowed ? (
        <Text variant="caption" tone="muted">
          Öneriler hakkında karar verme yetkiniz yok.
        </Text>
      ) : null}
      {done ? <Button title="Kuyruğa dön" variant="secondary" onPress={() => router.back()} /> : null}

      <ReasonSheet
        mode={sheet}
        busy={d.isPending}
        error={d.error && sheet ? errorMessage(d.error) : null}
        onCancel={() => setSheet(null)}
        onSubmit={({ reason, accountCode }) =>
          void (sheet?.kind === 'correct' && accountCode
            ? run({ action: 'correct', reason, accountCode }, `${accountCode} olarak düzeltildi ve uygulandı.`)
            : run({ action: 'reject', reason }, 'Öneri reddedildi.'))
        }
      />
    </Screen>
  );
}

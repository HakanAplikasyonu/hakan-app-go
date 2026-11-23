import { Image } from 'expo-image';
import { useLocalSearchParams, useRouter } from 'expo-router';
import { useState } from 'react';
import { ActivityIndicator, Pressable, View } from 'react-native';

import { Button, Card, Chip, Notice, Screen, Text } from '@/components/ui';
import { errorMessage } from '@/core/api/envelope';
import { useAuth, useOrganization } from '@/core/auth/auth-provider';
import { useColors } from '@/core/theme';
import { AccountPicker, type PickedAccount } from '@/features/accounts/account-picker';
import { approveDecision } from '@/features/accounts/filter';
import { receiptImageSource, useTaxpayerName } from '@/features/clients/api';
import { money, quantity } from '@/features/clients/format';
import { useOpenReceipt } from '@/features/files/receipt';
import { finishReceiptExpense, useDecide, useInvalidateEngagement, useReceiptExpense, useSuggestion, useSuggestionContext } from '@/features/review/api';
import { confidenceBand, confidencePercent, fieldLabel, proposedCode, proposedText, receiptApproveMessage } from '@/features/review/format';
import { ReasonSheet, type ReasonSheetMode } from '@/features/review/reason-sheet';
import { periodLabel } from '@/features/warnings/format';

/** Önerinin kaynağı kullanıcı diliyle; kural anahtarı gösterilmez (#378). */
const KAYNAK: Record<string, string> = { rule: 'kural', model: 'yapay zekâ', document: 'belge', input: 'girdi' };

/**
 * Öneri ayrıntısı (#239): belge, önerilen hesap kodu ve gerekçe; onayla / düzelt / reddet.
 * Oğuz'un canlı geri bildirimi: mükellef unvanı, fiş görseli tam ekran,
 * hesap kodu hesap planı seçicisiyle değiştirilip Onayla ile gönderilir.
 */
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
  const [chosen, setChosen] = useState<PickedAccount | null>(null);
  const [picking, setPicking] = useState(false);
  const taxpayer = useTaxpayerName(engagement);
  const receipt = useOpenReceipt();

  const sug = s.data;
  const code = sug ? proposedCode(sug.proposed_value) : null;
  const pending = sug?.status === 'pending';
  const allowed = can('tenant.review.decide');
  const doc = ctx.data;
  // Fiş önerisinde masraf taslağı: önerilen hesap kodu buradan değiştirilir.
  const expense = useReceiptExpense(engagement, doc?.receipt?.uuid).data ?? null;
  const receiptPending = expense?.approval_status === 'pending';
  const invalidateEngagement = useInvalidateEngagement();
  const [finishError, setFinishError] = useState<string | null>(null);

  async function run(decision: Parameters<typeof d.mutateAsync>[0]['decision'], message: string, after?: () => Promise<string>) {
    if (!sug) return;
    setFinishError(null);
    try {
      await d.mutateAsync({ suggestion: { uuid: sug.uuid, engagement_uuid: engagement }, decision });
    } catch {
      return; // d.error gösterilir
    }
    setSheet(null);
    void s.refetch();
    try {
      setDone(after ? await after() : message);
    } catch (e) {
      setDone(message);
      setFinishError(`Fiş muhasebeleşmedi: ${errorMessage(e)}`);
    }
  }

  function approve() {
    if (sug && expense && receiptPending) {
      const pick = chosen?.code ?? null;
      void run({ action: 'accept' }, 'Fiş alanı onaylandı.', async () => {
        const r = await finishReceiptExpense(org, { ...sug, engagement_uuid: engagement }, expense, pick);
        invalidateEngagement(engagement);
        return receiptApproveMessage(r, pick ?? expense.account_code);
      });
      return;
    }
    const dec = approveDecision(code, chosen?.code);
    void (dec.action === 'accept'
      ? run({ action: 'accept' }, `${code ?? 'Öneri'} onaylandı.`)
      : run({ action: 'correct', reason: dec.reason, accountCode: dec.accountCode }, `${dec.accountCode} hesabıyla onaylandı.`));
  }

  return (
    <Screen>
      {s.isLoading ? <ActivityIndicator color={c.brand} /> : null}
      {s.error ? <Notice>{errorMessage(s.error)}</Notice> : null}

      <View testID="review-taxpayer" style={{ gap: 2 }}>
        <Text variant="caption" tone="muted" style={{ fontWeight: '600' }}>
          Mükellef
        </Text>
        <Text variant="heading">{taxpayer ?? '—'}</Text>
      </View>

      {doc?.receipt && doc.receipt.media_type.startsWith('image/') ? (
        <Pressable
          testID="review-receipt-thumb"
          accessibilityRole="imagebutton"
          accessibilityLabel="Fiş görselini tam ekran aç"
          onPress={() => doc.receipt && receipt.open({ engagement, uuid: doc.receipt.uuid, mediaType: doc.receipt.media_type, title: taxpayer ?? undefined })}>
          <Image
            source={receiptImageSource(org, engagement, doc.receipt.uuid)}
            style={{ width: '100%', aspectRatio: 3 / 4, borderRadius: 16, backgroundColor: c.border }}
            contentFit="contain"
          />
        </Pressable>
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
                {quantity(doc.item.quantity, doc.item.unit_name, doc.item.unit_code)} · {money(doc.item.line_amount, doc.invoice.currency)} · KDV %{Number(doc.item.tax_rate)}
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
          <View style={{ flexDirection: 'row', alignItems: 'center', gap: 10 }}>
            <Text variant="title" testID={code !== null ? 'review-account-code' : undefined} style={{ flex: 1 }}>
              {(code !== null ? chosen?.code : null) ?? (proposedText(sug.proposed_value) || '—')}
            </Text>
            {pending && allowed && code !== null ? (
              <Button testID="review-account-edit" title="Değiştir" variant="secondary" onPress={() => setPicking(true)} disabled={d.isPending} />
            ) : null}
          </View>
          {chosen && code !== null ? (
            <Text variant="caption" tone="brand">
              {chosen.name ? `${chosen.name} · ` : ''}Önerilen {code} yerine seçildi; Onayla bu kodu gönderir.
            </Text>
          ) : null}
          <Text variant="caption" tone="muted">
            Gerekçe
          </Text>
          <Text>{sug.rationale}</Text>
          <Text variant="caption" tone="muted">
            Kaynak: {KAYNAK[sug.provenance.origin] ?? sug.provenance.origin}
          </Text>
          {!pending ? <Chip label={`Durum: ${sug.status}`} /> : null}
        </Card>
      ) : null}

      {expense ? (
        <Card>
          <Text variant="caption" tone="muted" style={{ fontWeight: '600' }}>
            {chosen ? 'Seçilen hesap kodu' : 'Fişin önerilen hesap kodu'}
          </Text>
          <View style={{ flexDirection: 'row', alignItems: 'center', gap: 10 }}>
            <Text variant="title" testID="review-account-code" style={{ flex: 1 }}>
              {chosen?.code ?? expense.account_code ?? '—'}
            </Text>
            {pending && allowed && receiptPending ? (
              <Button testID="review-account-edit" title="Değiştir" variant="secondary" onPress={() => setPicking(true)} disabled={d.isPending} />
            ) : null}
          </View>
          <Text variant="caption" tone="muted">
            {chosen ? (chosen.name ?? 'Elle girilen kod') : (expense.category_name ?? '')}
            {receiptPending ? ' · Onayla bu kodu masrafa yazar; fişin son alanıysa fiş onaylanır.' : ' · Fiş zaten karara bağlanmış.'}
          </Text>
        </Card>
      ) : null}

      {done ? <Notice tone="info">{done}</Notice> : null}
      {finishError ? <Notice>{finishError}</Notice> : null}
      {d.error && !sheet ? <Notice>{errorMessage(d.error)}</Notice> : null}

      {sug && pending && allowed ? (
        <View style={{ gap: 10 }}>
          <Button title="Onayla" testID="review-detail-approve" onPress={approve} loading={d.isPending && !sheet} disabled={d.isPending} />
          <Button title="Düzelt" variant="secondary" onPress={() => setSheet({ kind: 'correct', initialCode: (code !== null ? chosen?.code : null) ?? code ?? '' })} disabled={d.isPending} />
          <Button title="Reddet" variant="ghost" onPress={() => setSheet({ kind: 'reject' })} disabled={d.isPending} />
        </View>
      ) : null}
      {sug && pending && !allowed ? (
        <Text variant="caption" tone="muted">
          Öneriler hakkında karar verme yetkiniz yok.
        </Text>
      ) : null}
      {done ? <Button title="Kuyruğa dön" variant="secondary" onPress={() => router.back()} /> : null}

      <AccountPicker
        engagement={engagement}
        visible={picking}
        current={chosen?.code ?? (expense ? expense.account_code : code)}
        onClose={() => setPicking(false)}
        onPick={(a) => {
          setChosen(a);
          setPicking(false);
        }}
      />

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

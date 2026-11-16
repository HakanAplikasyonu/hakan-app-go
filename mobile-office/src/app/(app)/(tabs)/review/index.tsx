import { BosDurum, GuvenOlcegi, KristalYukleyici, TacRozeti, type GuvenKaynagi } from '@hakan/mobile-core/hakan';
import { Ikon, type IkonAdi } from '@hakan/mobile-core/ui/ikon';
import { DOKUNMA, yerlesim } from '@hakan/mobile-core/ui/layout';
import { EkranBasligi, Hap, IlerlemeCubugu, KesikKose, metin } from '@hakan/mobile-core/ui/parcalar';
import { useTheme } from '@hakan/mobile-core/ui/theme';
import { Image } from 'expo-image';
import { useRouter } from 'expo-router';
import { useCallback, useMemo, useState } from 'react';
import { FlatList, Pressable, RefreshControl, ScrollView, StyleSheet, Text, useWindowDimensions, View } from 'react-native';
import { SafeAreaView } from 'react-native-safe-area-context';

import { Notice } from '@/components/ui';
import { errorMessage } from '@/core/api/envelope';
import { useAuth, useOrganization } from '@/core/auth/auth-provider';
import { receiptImageSource } from '@/features/clients/api';
import { money } from '@/features/clients/format';
import { useDecide, useReviewQueue, useSuggestionContext, type Suggestion, type SuggestionContext } from '@/features/review/api';
import { confidencePercent, fieldLabel, proposedCode, queueEta, queueProgress } from '@/features/review/format';
import { KaydirmaKarti } from '@/features/review/kaydirma-karti';
import { ReasonSheet, type ReasonSheetMode } from '@/features/review/reason-sheet';
import { periodLabel } from '@/features/warnings/format';

const KAYNAK: Record<string, GuvenKaynagi> = { rule: 'kural', document: 'gecmis', model: 'model', input: 'girdi' };
/** Önerinin kaynağı → güven ölçeği kaynağı (kural eşleşmesi her zaman yüksek). */
const kaynakOf = (s: Suggestion): GuvenKaynagi | undefined => KAYNAK[s.provenance?.origin ?? ''];

function docKind(ctx: SuggestionContext | undefined): string {
  if (ctx?.receipt) return 'Fiş';
  if (ctx?.invoice) return ctx.invoice.direction === 'incoming' ? 'Gelen e-Fatura' : 'Giden e-Fatura';
  return 'Öneri';
}

/**
 * Onay kuyruğu (MusavirMobil · 2): kart yığını, sağa onay, sola red, yukarı
 * düzelt; aynı eylemler alt düğmelerde (erişilebilirlik). Üstte ilerleme
 * çubuğu. Tablette (MusavirTablet) liste + belge + karar paneli.
 */
export default function ReviewScreen() {
  const { colors: c } = useTheme();
  const { can } = useAuth();
  const allowed = can('tenant.review.decide');
  const { width, height } = useWindowDimensions();
  const tablet = yerlesim(width) !== 'telefon';
  const q = useReviewQueue();
  const decide = useDecide();
  const items = useMemo(() => (q.data?.pages.flatMap((p) => p.items) ?? []).filter((s) => s.status === 'pending'), [q.data]);
  const total = q.data?.pages[0]?.total ?? 0;

  const [decided, setDecided] = useState(0);
  const [skipped, setSkipped] = useState<Set<string>>(new Set());
  const [selected, setSelected] = useState<string | null>(null);
  const [sheet, setSheet] = useState<{ s: Suggestion; mode: ReasonSheetMode } | null>(null);
  const [notice, setNotice] = useState<{ tone: 'info' | 'danger'; text: string } | null>(null);
  const [approving, setApproving] = useState<string | null>(null);
  const [attempt, setAttempt] = useState(0);

  const queue = items.filter((s) => !skipped.has(s.uuid));
  const current = (tablet && selected ? queue.find((s) => s.uuid === selected) : null) ?? queue[0] ?? null;
  const remaining = Math.max(0, total - decided);

  const run = useCallback(
    async (s: Suggestion, decision: Parameters<typeof decide.mutateAsync>[0]['decision'], ok: string) => {
      setNotice(null);
      try {
        await decide.mutateAsync({ suggestion: s, decision });
        setSkipped((x) => new Set(x).add(s.uuid));
        setDecided((n) => n + 1);
        setNotice({ tone: 'info', text: ok });
        setSheet(null);
        return true;
      } catch (e) {
        setNotice({ tone: 'danger', text: errorMessage(e) });
        setAttempt((n) => n + 1); // kart yerine döner
        return false;
      } finally {
        setApproving(null);
      }
    },
    [decide],
  );

  const approve = (s: Suggestion) => {
    setApproving(s.uuid);
    void run(s, { action: 'accept' }, `${proposedCode(s.proposed_value) ?? 'Öneri'} onaylandı.`);
  };
  const reject = (s: Suggestion) => setSheet({ s, mode: { kind: 'reject' } });
  const correct = (s: Suggestion) => setSheet({ s, mode: { kind: 'correct', initialCode: proposedCode(s.proposed_value) ?? '' } });

  const header = (
    <>
      <EkranBasligi baslik="Onay" sag={q.data ? <Hap metin={`${remaining} kaldı`} testID="review-remaining" /> : null} />
      <View style={{ paddingHorizontal: 16, gap: 6 }}>
        <IlerlemeCubugu oran={queueProgress(decided, remaining)} etiket={`${decided} karar verildi, ${remaining} öneri kaldı`} />
        {remaining ? <Text style={[metin.xs, { color: c.text3 }]}>Ortalama 6 sn / öneri · {queueEta(remaining)}</Text> : null}
      </View>
      {notice ? (
        <View style={{ paddingHorizontal: 16, paddingTop: 10 }}>
          {notice.tone === 'info' ? (
            <View accessibilityLiveRegion="polite" style={[styles.toast, { backgroundColor: c.text }]}>
              <TacRozeti durum="onaylandi" />
              <Text style={[metin.sm, { color: c.bg, flex: 1 }]}>{notice.text}</Text>
            </View>
          ) : (
            <Notice>{notice.text}</Notice>
          )}
        </View>
      ) : null}
      {!allowed && q.data ? (
        <Text style={[metin.xs, { color: c.text3, paddingHorizontal: 16, paddingTop: 8 }]}>Karar yetkiniz yok, yalnızca görüntüleme.</Text>
      ) : null}
    </>
  );

  const empty = q.isLoading ? (
    <View style={{ alignItems: 'center', paddingVertical: 48 }}>
      <KristalYukleyici etiket="Kuyruk yükleniyor" />
    </View>
  ) : q.error ? (
    <View style={{ padding: 16 }}>
      <Notice>{errorMessage(q.error)}</Notice>
    </View>
  ) : (
    <BosDurum baslik="Kuyruk boş" aciklama="Bekleyen öneri yok. Yeni belge geldiğinde öneriler burada sıralanır." />
  );

  const sheetEl = (
    <ReasonSheet
      mode={sheet?.mode ?? null}
      busy={decide.isPending}
      error={null}
      onCancel={() => {
        setSheet(null);
        setAttempt((n) => n + 1);
      }}
      onSubmit={({ reason, accountCode }) => {
        if (!sheet) return;
        const s = sheet.s;
        void (sheet.mode.kind === 'correct'
          ? run(s, { action: 'correct', reason, accountCode: accountCode ?? '' }, `${accountCode} olarak düzeltildi.`)
          : run(s, { action: 'reject', reason }, 'Öneri reddedildi.'));
      }}
    />
  );

  if (tablet) {
    return (
      <SafeAreaView style={{ flex: 1, backgroundColor: c.bg }} edges={['top', 'right']}>
        <View style={{ flex: 1, flexDirection: 'row' }}>
          <View style={[styles.listPane, { backgroundColor: c.surface, borderRightColor: c.line }]}>
            {header}
            <FlatList
              data={queue}
              keyExtractor={(s) => s.uuid}
              contentContainerStyle={{ paddingTop: 10, paddingBottom: 24 }}
              onEndReached={() => q.hasNextPage && !q.isFetchingNextPage && q.fetchNextPage()}
              ListEmptyComponent={empty}
              renderItem={({ item: s }) => {
                const on = s.uuid === current?.uuid;
                return (
                  <Pressable
                    testID="suggestion-row"
                    accessibilityRole="button"
                    accessibilityState={{ selected: on }}
                    accessibilityLabel={`${fieldLabel(s.target_field)} ${proposedCode(s.proposed_value) ?? ''}, güven ${confidencePercent(s.confidence)}`}
                    onPress={() => setSelected(s.uuid)}
                    style={[styles.listRow, { borderTopColor: c.line }, on ? { backgroundColor: c.brandTint, borderLeftColor: c.brand, borderLeftWidth: 3 } : null]}>
                    <GuvenOlcegi deger={Number(s.confidence)} kaynak={kaynakOf(s)} />
                    <View style={{ flex: 1, gap: 2 }}>
                      <Text style={[metin.sm, { fontFamily: 'Inter_600SemiBold', color: c.text }]} numberOfLines={1}>
                        {fieldLabel(s.target_field)}
                        {s.period ? ` · ${periodLabel(s.period)}` : ''}
                      </Text>
                      <Text style={[metin.xs, metin.mono, { color: c.text3 }]}>{proposedCode(s.proposed_value) ?? '—'}</Text>
                    </View>
                  </Pressable>
                );
              }}
            />
          </View>
          {current ? (
            <TabletDetail
              s={current}
              allowed={allowed}
              busy={decide.isPending}
              approving={approving === current.uuid}
              onApprove={() => approve(current)}
              onReject={() => reject(current)}
              onCorrect={() => correct(current)}
            />
          ) : (
            <View style={{ flex: 1, justifyContent: 'center' }}>{q.data ? empty : null}</View>
          )}
        </View>
        {sheetEl}
      </SafeAreaView>
    );
  }

  const cardH = Math.max(320, Math.min(430, height * 0.5));
  const cardW = width - 32;
  return (
    <SafeAreaView style={{ flex: 1, backgroundColor: c.bg }} edges={['top', 'left', 'right']}>
      <ScrollView
        contentContainerStyle={{ paddingBottom: 24 }}
        refreshControl={<RefreshControl refreshing={q.isRefetching && !q.isFetchingNextPage} onRefresh={() => void q.refetch()} tintColor={c.brandText} />}>
        {header}
        <View style={{ paddingHorizontal: 16, paddingTop: 18, gap: 16 }}>
          {current ? (
            <>
              <View style={{ height: cardH + 10 }}>
                {queue[1] ? <View style={[styles.behind, { backgroundColor: c.surface, borderColor: c.line, height: cardH - 12 }]} /> : null}
                <KaydirmaKarti
                  key={current.uuid}
                  cardKey={`${current.uuid}:${attempt}`}
                  testID="suggestion-card"
                  width={cardW}
                  height={cardH}
                  enabled={allowed && !decide.isPending}
                  colors={{ ok: c.ok, onOk: c.surface, danger: c.danger, onDanger: c.surface, brand: c.brand, onBrand: c.onBrand }}
                  onApprove={() => approve(current)}
                  onReject={() => reject(current)}
                  onEdit={() => correct(current)}>
                  <CardBody s={current} approving={approving === current.uuid} />
                </KaydirmaKarti>
              </View>
              {allowed ? (
                <View style={styles.actions}>
                  <RoundButton testID="review-reject" label="Reddet" ikon="kapat" size={60} fg={c.danger} bg={c.surface} border={c.lineStrong} onPress={() => reject(current)} />
                  <RoundButton testID="review-correct" label="Düzelt" ikon="duzelt" size={52} fg={c.text} bg={c.surface} border={c.lineStrong} onPress={() => correct(current)} />
                  <RoundButton
                    testID="review-approve"
                    label="Onayla"
                    ikon="onayla"
                    size={60}
                    fg={c.surface}
                    bg={c.ok}
                    onPress={() => approve(current)}
                    disabled={decide.isPending}
                  />
                </View>
              ) : null}
            </>
          ) : (
            empty
          )}
        </View>
      </ScrollView>
      {sheetEl}
    </SafeAreaView>
  );
}

function CardBody({ s, approving }: { s: Suggestion; approving: boolean }) {
  const { colors: c } = useTheme();
  const router = useRouter();
  const ctx = useSuggestionContext(s.engagement_uuid ?? '', s.uuid);
  const d = ctx.data;
  const amount = d?.invoice ? money(d.item?.line_amount ?? d.invoice.payable_amount, d.invoice.currency) : null;
  const code = proposedCode(s.proposed_value);
  return (
    <Pressable
      accessibilityRole="button"
      accessibilityHint="Ayrıntıyı açar. Sağa kaydır onayla, sola kaydır reddet, yukarı kaydır düzelt."
      accessibilityLabel={`${docKind(d)}, ${d?.invoice?.counterparty_name ?? ''}, ${fieldLabel(s.target_field)} ${code ?? ''}, güven ${confidencePercent(s.confidence)}`}
      onPress={() => router.push({ pathname: '/review/[uuid]', params: { uuid: s.uuid, engagement: s.engagement_uuid ?? '' } })}
      style={[styles.card, { backgroundColor: c.surface, borderColor: c.line }]}>
      <View style={{ flexDirection: 'row', alignItems: 'center', gap: 8 }}>
        <Hap metin={docKind(d)} ton="brand" />
        <View style={{ flex: 1 }} />
        <TacRozeti durum={approving ? 'onaylandi' : 'yuklendi'} />
        {s.period ? <Text style={[metin.xs, { color: c.text3 }]}>{periodLabel(s.period)}</Text> : null}
      </View>
      <View style={{ gap: 4 }}>
        <Text style={[metin.baslik2, { color: c.text }]} numberOfLines={2}>
          {d?.invoice?.counterparty_name || (d?.receipt ? 'Fiş görseli' : fieldLabel(s.target_field))}
        </Text>
        {d?.item ? (
          <Text style={[metin.sm, { color: c.text2 }]} numberOfLines={2}>
            {d.item.name} · {d.item.quantity} {d.item.unit_code ?? ''}
          </Text>
        ) : null}
      </View>
      {amount ? <Text style={[metin.kpi, { fontSize: 28, lineHeight: 34, color: c.text }]}>{amount}</Text> : null}
      <View style={[styles.codeBox, { backgroundColor: c.surface2, borderColor: c.line }]}>
        <View style={{ flexDirection: 'row', alignItems: 'center', gap: 8 }}>
          <Text style={[metin.mono, { fontSize: 15, color: c.brandText }]}>{code ?? '—'}</Text>
          <Text style={[metin.sm, { fontFamily: 'Inter_600SemiBold', color: c.text }]}>{fieldLabel(s.target_field)}</Text>
        </View>
        <View style={{ flexDirection: 'row', alignItems: 'center', gap: 8 }}>
          <GuvenOlcegi deger={Number(s.confidence)} kaynak={kaynakOf(s)} />
          <Text style={[metin.xs, { color: c.text2, flex: 1 }]} numberOfLines={2}>
            {s.rationale}
          </Text>
        </View>
      </View>
      <View style={{ flex: 1 }} />
      <View style={styles.hints} importantForAccessibility="no-hide-descendants">
        <Text style={[metin.xs, { color: c.text3 }]}>← reddet</Text>
        <Text style={[metin.xs, { color: c.text3 }]}>yukarı: düzelt</Text>
        <Text style={[metin.xs, { color: c.text3 }]}>onayla →</Text>
      </View>
      <KesikKose zemin={c.bg} />
    </Pressable>
  );
}

function TabletDetail({
  s,
  allowed,
  busy,
  approving,
  onApprove,
  onReject,
  onCorrect,
}: {
  s: Suggestion;
  allowed: boolean;
  busy: boolean;
  approving: boolean;
  onApprove: () => void;
  onReject: () => void;
  onCorrect: () => void;
}) {
  const { colors: c } = useTheme();
  const org = useOrganization();
  const ctx = useSuggestionContext(s.engagement_uuid ?? '', s.uuid);
  const d = ctx.data;
  const code = proposedCode(s.proposed_value);
  return (
    <>
      <View style={[styles.docPane, { backgroundColor: c.surface3 }]}>
        {d?.receipt && d.receipt.media_type.startsWith('image/') ? (
          <Image
            source={receiptImageSource(org, s.engagement_uuid ?? '', d.receipt.uuid)}
            style={{ width: '80%', height: '80%' }}
            contentFit="contain"
            accessibilityLabel="Fiş görseli"
          />
        ) : d?.invoice ? (
          <View style={[styles.paper, { backgroundColor: c.surface, borderColor: c.line }]}>
            <Text style={[metin.cap, { color: c.text3 }]}>{d.invoice.invoice_number}</Text>
            <Text style={[metin.baslik2, { color: c.text }]}>{d.invoice.counterparty_name || '—'}</Text>
            <Text style={[metin.sm, { color: c.text2 }]}>{d.invoice.issue_date}</Text>
            <Text style={[metin.kpi, { fontSize: 24, color: c.text }]}>{money(d.invoice.payable_amount, d.invoice.currency)}</Text>
          </View>
        ) : ctx.isLoading ? (
          <KristalYukleyici etiket="Belge yükleniyor" />
        ) : (
          <Text style={[metin.sm, { color: c.text3 }]}>Belge görseli yok</Text>
        )}
      </View>
      <ScrollView style={[styles.decision, { backgroundColor: c.surface, borderLeftColor: c.line }]} contentContainerStyle={{ padding: 22, gap: 16, flexGrow: 1 }}>
        <Hap metin={docKind(d)} ton="brand" />
        <View style={{ gap: 4 }}>
          <Text style={[metin.baslik2, { color: c.text }]}>{d?.invoice?.counterparty_name || fieldLabel(s.target_field)}</Text>
          <Text style={[metin.sm, { color: c.text2 }]}>{[periodLabel(s.period ?? null), d?.invoice?.issue_date].filter(Boolean).join(' · ')}</Text>
        </View>
        {d?.item ? (
          <View style={[styles.codeBox, { borderColor: c.line }]}>
            <Row k="Kalem" v={d.item.name || '—'} />
            <Row k="Tutar" v={money(d.item.line_amount, d.invoice?.currency ?? 'TRY')} num />
            <Row k="KDV" v={`%${Number(d.item.tax_rate)}`} num />
          </View>
        ) : null}
        <View style={[styles.codeBox, { borderColor: c.brand }]}>
          <View style={{ flexDirection: 'row', alignItems: 'center', gap: 8 }}>
            <Text style={[metin.mono, { fontSize: 15, color: c.brandText }]}>{code ?? '—'}</Text>
            <Text style={[metin.sm, { fontFamily: 'Inter_600SemiBold', color: c.text }]}>{fieldLabel(s.target_field)}</Text>
          </View>
          <View style={{ flexDirection: 'row', alignItems: 'center', gap: 8 }}>
            <GuvenOlcegi deger={Number(s.confidence)} kaynak={kaynakOf(s)} />
            <Text style={[metin.xs, { color: c.text2, flex: 1 }]}>
              {confidencePercent(s.confidence)} · {s.rationale}
            </Text>
          </View>
        </View>
        <View style={{ flex: 1 }} />
        {allowed ? (
          <>
            <Pressable
              testID="review-approve"
              accessibilityRole="button"
              disabled={busy}
              onPress={onApprove}
              style={({ pressed }) => [styles.bigBtn, { flex: 0, backgroundColor: c.ok, opacity: busy ? 0.6 : pressed ? 0.85 : 1 }]}>
              {approving ? <TacRozeti durum="onaylandi" /> : null}
              <Text style={[styles.bigBtnText, { color: c.surface }]}>Onayla</Text>
            </Pressable>
            <View style={{ flexDirection: 'row', gap: 8 }}>
              <Pressable testID="review-correct" accessibilityRole="button" onPress={onCorrect} style={[styles.bigBtn, styles.outline, { borderColor: c.lineStrong }]}>
                <Text style={[styles.bigBtnText, { color: c.text }]}>Düzelt</Text>
              </Pressable>
              <Pressable testID="review-reject" accessibilityRole="button" onPress={onReject} style={[styles.bigBtn, styles.outline, { borderColor: c.lineStrong }]}>
                <Text style={[styles.bigBtnText, { color: c.danger }]}>Reddet</Text>
              </Pressable>
            </View>
          </>
        ) : null}
      </ScrollView>
    </>
  );
}

function Row({ k, v, num }: { k: string; v: string; num?: boolean }) {
  const { colors: c } = useTheme();
  return (
    <View style={{ flexDirection: 'row', gap: 8 }}>
      <Text style={[metin.sm, { color: c.text3, flex: 1 }]}>{k}</Text>
      <Text style={[metin.sm, num ? metin.num : null, { color: c.text, fontFamily: 'Inter_600SemiBold' }]}>{v}</Text>
    </View>
  );
}

function RoundButton({
  label,
  ikon,
  size,
  fg,
  bg,
  border,
  onPress,
  disabled,
  testID,
}: {
  label: string;
  ikon: IkonAdi;
  size: number;
  fg: string;
  bg: string;
  border?: string;
  onPress: () => void;
  disabled?: boolean;
  testID?: string;
}) {
  return (
    <Pressable
      testID={testID}
      accessibilityRole="button"
      accessibilityLabel={label}
      disabled={disabled}
      onPress={onPress}
      hitSlop={Math.max(0, (DOKUNMA - size) / 2)}
      style={({ pressed }) => ({
        width: size,
        height: size,
        borderRadius: size / 2,
        backgroundColor: bg,
        borderWidth: border ? 1.5 : 0,
        borderColor: border,
        alignItems: 'center',
        justifyContent: 'center',
        opacity: disabled ? 0.5 : pressed ? 0.8 : 1,
      })}>
      <Ikon ad={ikon} boyut={size >= 60 ? 22 : 18} renk={fg} />
    </Pressable>
  );
}

const styles = StyleSheet.create({
  toast: { flexDirection: 'row', alignItems: 'center', gap: 10, borderRadius: 12, paddingHorizontal: 14, paddingVertical: 12 },
  behind: { position: 'absolute', left: 14, right: 14, top: 22, borderRadius: 22, borderWidth: StyleSheet.hairlineWidth, opacity: 0.6 },
  card: { flex: 1, borderRadius: 22, borderWidth: StyleSheet.hairlineWidth, padding: 20, gap: 14, overflow: 'hidden', shadowColor: '#0A0A0A', shadowOpacity: 0.14, shadowRadius: 20, shadowOffset: { width: 0, height: 12 }, elevation: 6 },
  codeBox: { borderWidth: 1, borderRadius: 14, padding: 14, gap: 8 },
  hints: { flexDirection: 'row', justifyContent: 'space-between' },
  actions: { flexDirection: 'row', justifyContent: 'center', alignItems: 'center', gap: 18 },
  listPane: { width: 340, borderRightWidth: StyleSheet.hairlineWidth },
  listRow: { flexDirection: 'row', alignItems: 'center', gap: 10, paddingHorizontal: 20, paddingVertical: 12, minHeight: 56, borderTopWidth: StyleSheet.hairlineWidth },
  docPane: { flex: 1, alignItems: 'center', justifyContent: 'center' },
  paper: { width: 300, padding: 24, gap: 8, borderRadius: 4, borderWidth: StyleSheet.hairlineWidth },
  decision: { width: 360, flexGrow: 0, borderLeftWidth: StyleSheet.hairlineWidth },
  bigBtn: { flex: 1, flexDirection: 'row', gap: 8, minHeight: 52, borderRadius: 12, alignItems: 'center', justifyContent: 'center', paddingHorizontal: 16 },
  outline: { borderWidth: 1 },
  bigBtnText: { fontFamily: 'Inter_600SemiBold', fontSize: 16 },
});

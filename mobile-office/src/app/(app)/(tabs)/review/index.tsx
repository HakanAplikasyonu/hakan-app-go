import { BosDurum, GuvenOlcegi, KristalYukleyici, TacRozeti, type GuvenKaynagi } from '@hakan/mobile-core/hakan';
import { Ikon, type IkonAdi } from '@hakan/mobile-core/ui/ikon';
import { DOKUNMA, yerlesim } from '@hakan/mobile-core/ui/layout';
import { EkranBasligi, Hap, IlerlemeCubugu, KesikKose, metin, Segment } from '@hakan/mobile-core/ui/parcalar';
import { useTheme } from '@hakan/mobile-core/ui/theme';
import { Image } from 'expo-image';
import { useRouter } from 'expo-router';
import { useCallback, useMemo, useState } from 'react';
import { Pressable, SectionList, RefreshControl, ScrollView, StyleSheet, Text, useWindowDimensions, View } from 'react-native';
import { SafeAreaView } from 'react-native-safe-area-context';

import { Notice } from '@/components/ui';
import { errorMessage } from '@/core/api/envelope';
import { useAuth, useOrganization } from '@/core/auth/auth-provider';
import { AccountPicker, type PickedAccount } from '@/features/accounts/account-picker';
import { approveDecision } from '@/features/accounts/filter';
import { receiptImageSource, useClientTitles, useTaxpayerName } from '@/features/clients/api';
import { money, quantity } from '@/features/clients/format';
import { useOpenReceipt } from '@/features/files/receipt';
import {
  finishReceiptExpense,
  useDecide,
  useInvalidateEngagement,
  useReceiptExpense,
  useReviewQueue,
  useSuggestionContext,
  type ReceiptExpense,
  type Suggestion,
  type SuggestionContext,
} from '@/features/review/api';
import {
  confidencePercent,
  groupQueue,
  proposedCode,
  proposedText,
  queueEta,
  queueProgress,
  receiptApproveMessage,
  suggestionLabel,
  type QueueGrouping,
} from '@/features/review/format';
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
 *
 * Oğuz'un canlı geri bildirimi: her kartta mükellefin unvanı ve fiş küçük
 * görseli (dokununca tam ekran); önerilen hesap kodu hesap planı
 * seçicisiyle değiştirilir, Onayla seçilen kodu gönderir (değişmediyse
 * `accept`, değiştiyse seçilen kodla `correct`).
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
  // Onaydan önce hesap planından seçilen kod (öneri başına).
  const [chosen, setChosen] = useState<Record<string, PickedAccount>>({});
  const [picker, setPicker] = useState<{ s: Suggestion; current: string | null } | null>(null);
  const org = useOrganization();
  const invalidateEngagement = useInvalidateEngagement();

  const queue = items.filter((s) => !skipped.has(s.uuid));
  const [grouping, setGrouping] = useState<QueueGrouping>('mukellef');
  const clients = useClientTitles();
  const sections = useMemo(() => {
    if (!tablet) return [];
    const titles = new Map((clients ?? []).map((x) => [x.engagement_uuid, x.title]));
    return groupQueue(queue, grouping, (e) => titles.get(e));
  }, [tablet, queue, grouping, clients]);
  // Tablette seçim yoksa görünen listenin ilk satırı (gruplama sırası).
  const firstShown = sections[0]?.data[0] ?? null;
  const current = (tablet && selected ? queue.find((s) => s.uuid === selected) : null) ?? (tablet ? firstShown : null) ?? queue[0] ?? null;
  const remaining = Math.max(0, total - decided);
  // Fiş kartında masraf taslağı (önerilen hesap kodu); kartla aynı önbellek.
  const curCtx = useSuggestionContext(current?.engagement_uuid ?? '', current?.uuid ?? '', !!current);
  const curExpense = useReceiptExpense(current?.engagement_uuid, curCtx.data?.receipt?.uuid).data ?? null;

  const run = useCallback(
    async (s: Suggestion, decision: Parameters<typeof decide.mutateAsync>[0]['decision'], ok: string, after?: () => Promise<string>) => {
      setNotice(null);
      try {
        await decide.mutateAsync({ suggestion: s, decision });
      } catch (e) {
        setNotice({ tone: 'danger', text: errorMessage(e) });
        setAttempt((n) => n + 1); // kart yerine döner
        setApproving(null);
        return false;
      }
      setSkipped((x) => new Set(x).add(s.uuid));
      setDecided((n) => n + 1);
      setSheet(null);
      try {
        setNotice({ tone: 'info', text: after ? await after() : ok });
      } catch (e) {
        // Öneri karara bağlandı; fişin hesabı / onayı yazılamadı.
        setNotice({ tone: 'danger', text: `${ok} Fiş muhasebeleşmedi: ${errorMessage(e)}` });
      } finally {
        setApproving(null);
      }
      return true;
    },
    [decide],
  );

  const approve = (s: Suggestion) => {
    setApproving(s.uuid);
    const expense = s.uuid === current?.uuid ? curExpense : null;
    if (expense && expense.approval_status === 'pending') {
      // Fiş: okunan alan onaylanır, seçilen hesap kodu masrafa yazılır, son alansa masraf onaylanır.
      const pick = chosen[s.uuid]?.code ?? null;
      void run(s, { action: 'accept' }, 'Fiş alanı onaylandı.', async () => {
        const r = await finishReceiptExpense(org, s, expense, pick);
        invalidateEngagement(s.engagement_uuid);
        return receiptApproveMessage(r, pick ?? expense.account_code);
      });
      return;
    }
    const d = approveDecision(proposedCode(s.proposed_value), chosen[s.uuid]?.code);
    void (d.action === 'accept'
      ? run(s, { action: 'accept' }, `${proposedText(s.proposed_value) || 'Öneri'} onaylandı.`)
      : run(s, { action: 'correct', reason: d.reason, accountCode: d.accountCode }, `${d.accountCode} hesabıyla onaylandı.`));
  };
  const editCode = (s: Suggestion, cur: string | null) => setPicker({ s, current: cur });
  const reject = (s: Suggestion) => setSheet({ s, mode: { kind: 'reject' } });
  const correct = (s: Suggestion) => setSheet({ s, mode: { kind: 'correct', initialCode: (proposedCode(s.proposed_value) !== null ? chosen[s.uuid]?.code : null) ?? proposedCode(s.proposed_value) ?? '' } });

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

  const pickerEl = (
    <AccountPicker
      engagement={picker?.s.engagement_uuid ?? ''}
      visible={!!picker}
      current={picker?.current ?? null}
      onClose={() => setPicker(null)}
      onPick={(a) => {
        if (picker) setChosen((x) => ({ ...x, [picker.s.uuid]: a }));
        setPicker(null);
      }}
    />
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
            <Segment
              testIDOnek="review-group"
              secenekler={[
                { key: 'mukellef', label: 'Mükellefe göre' },
                { key: 'guven', label: 'Güvene göre' },
              ]}
              secili={grouping}
              onSec={setGrouping}
              style={{ marginHorizontal: 16, marginTop: 12 }}
            />
            <SectionList
              sections={sections}
              keyExtractor={(s) => s.uuid}
              stickySectionHeadersEnabled
              contentContainerStyle={{ paddingTop: 6, paddingBottom: 24 }}
              onEndReached={() => q.hasNextPage && !q.isFetchingNextPage && q.fetchNextPage()}
              ListEmptyComponent={empty}
              renderSectionHeader={({ section }) => (
                <View accessibilityRole="header" style={[styles.groupHead, { backgroundColor: c.surface2, borderTopColor: c.line }]}>
                  <Text style={[metin.xs, { fontFamily: 'Inter_600SemiBold', color: c.text2, flex: 1 }]} numberOfLines={1}>
                    {section.title}
                  </Text>
                  <Text style={[metin.xs, metin.mono, { color: c.text3 }]}>{section.data.length}</Text>
                </View>
              )}
              renderItem={({ item: s }) => {
                const on = s.uuid === current?.uuid;
                return (
                  <Pressable
                    testID="suggestion-row"
                    accessibilityRole="button"
                    accessibilityState={{ selected: on }}
                    accessibilityLabel={`${suggestionLabel(s)} ${proposedCode(s.proposed_value) ?? ''}, güven ${confidencePercent(s.confidence)}`}
                    onPress={() => setSelected(s.uuid)}
                    style={[styles.listRow, { borderTopColor: c.line }, on ? { backgroundColor: c.brandTint, borderLeftColor: c.brand, borderLeftWidth: 3 } : null]}>
                    <GuvenOlcegi deger={Number(s.confidence)} kaynak={kaynakOf(s)} />
                    <View style={{ flex: 1, gap: 2 }}>
                      <Text style={[metin.sm, { fontFamily: 'Inter_600SemiBold', color: c.text }]} numberOfLines={1}>
                        {suggestionLabel(s)}
                        {s.period ? ` · ${periodLabel(s.period)}` : ''}
                      </Text>
                      <Text style={[metin.xs, metin.mono, { color: c.text3 }]}>{proposedText(s.proposed_value) || '—'}</Text>
                    </View>
                  </Pressable>
                );
              }}
            />
          </View>
          {current ? (
            <TabletDetail
              s={current}
              chosen={chosen[current.uuid] ?? null}
              onEditCode={(cur) => editCode(current, cur)}
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
        {pickerEl}
      </SafeAreaView>
    );
  }

  // Mükellef satırı ve fiş görseli için kart biraz daha uzun (Oğuz'un canlı geri bildirimi).
  const cardH = Math.max(380, Math.min(500, height * 0.58));
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
                  <CardBody s={current} approving={approving === current.uuid} chosen={chosen[current.uuid] ?? null} onEditCode={allowed ? (cur) => editCode(current, cur) : undefined} />
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
      {pickerEl}
    </SafeAreaView>
  );
}

function CardBody({
  s,
  approving,
  chosen,
  onEditCode,
}: {
  s: Suggestion;
  approving: boolean;
  chosen: PickedAccount | null;
  onEditCode?: (current: string | null) => void;
}) {
  const { colors: c } = useTheme();
  const router = useRouter();
  const org = useOrganization();
  const ctx = useSuggestionContext(s.engagement_uuid ?? '', s.uuid);
  const taxpayer = useTaxpayerName(s.engagement_uuid);
  const receipt = useOpenReceipt();
  const d = ctx.data;
  const amount = d?.invoice ? money(d.item?.line_amount ?? d.invoice.payable_amount, d.invoice.currency) : null;
  const image = d?.receipt && d.receipt.media_type.startsWith('image/') ? d.receipt : null;
  const expense = useReceiptExpense(s.engagement_uuid, d?.receipt?.uuid).data ?? null;
  const code = proposedCode(s.proposed_value);
  return (
    <Pressable
      accessibilityRole="button"
      accessibilityHint="Ayrıntıyı açar. Sağa kaydır onayla, sola kaydır reddet, yukarı kaydır düzelt."
      accessibilityLabel={`${taxpayer ?? ''}, ${docKind(d)}, ${d?.invoice?.counterparty_name ?? ''}, ${suggestionLabel(s)} ${chosen?.code ?? proposedText(s.proposed_value)}, güven ${confidencePercent(s.confidence)}`}
      onPress={() => router.push({ pathname: '/review/[uuid]', params: { uuid: s.uuid, engagement: s.engagement_uuid ?? '' } })}
      style={[styles.card, { backgroundColor: c.surface, borderColor: c.line }]}>
      <View style={{ flexDirection: 'row', alignItems: 'center', gap: 8 }}>
        <Hap metin={docKind(d)} ton="brand" />
        <View style={{ flex: 1 }} />
        <TacRozeti durum={approving ? 'onaylandi' : 'yuklendi'} />
        {s.period ? <Text style={[metin.xs, { color: c.text3 }]}>{periodLabel(s.period)}</Text> : null}
      </View>
      <TaxpayerLine name={taxpayer} />
      <View style={{ flexDirection: 'row', gap: 12, alignItems: 'flex-start' }}>
        {image ? (
          <Pressable
            testID="review-receipt-thumb"
            accessibilityRole="imagebutton"
            accessibilityLabel="Fiş görselini tam ekran aç"
            onPress={() => receipt.open({ engagement: s.engagement_uuid ?? '', uuid: image.uuid, mediaType: image.media_type, title: taxpayer ?? undefined })}>
            <Image
              source={receiptImageSource(org, s.engagement_uuid ?? '', image.uuid)}
              style={[styles.thumb, { backgroundColor: c.surface3 }]}
              contentFit="cover"
              transition={120}
            />
          </Pressable>
        ) : null}
        <View style={{ flex: 1, gap: 4 }}>
          <Text style={[metin.baslik2, { color: c.text }]} numberOfLines={2}>
            {d?.invoice?.counterparty_name || (d?.receipt ? 'Fiş' : suggestionLabel(s))}
          </Text>
          {d?.item ? (
            <Text style={[metin.sm, { color: c.text2 }]} numberOfLines={2}>
              {d.item.name} · {quantity(d.item.quantity, d.item.unit_name, d.item.unit_code)}
            </Text>
          ) : null}
          {image ? <Text style={[metin.xs, { color: c.text3 }]}>Büyütmek için görsele dokunun</Text> : null}
        </View>
      </View>
      {amount ? <Text style={[metin.kpi, { fontSize: 28, lineHeight: 34, color: c.text }]}>{amount}</Text> : null}
      {expense ? <ReceiptAccountBox expense={expense} chosen={chosen} onEdit={onEditCode} /> : null}
      <View style={[styles.codeBox, { backgroundColor: c.surface2, borderColor: chosen && code !== null ? c.brand : c.line }]}>
        <View style={{ flexDirection: 'row', alignItems: 'center', gap: 8 }}>
          <Text testID={code !== null ? 'review-account-code' : undefined} style={[metin.mono, { fontSize: 15, color: c.brandText }]}>
            {(code !== null ? chosen?.code : null) ?? (proposedText(s.proposed_value) || '—')}
          </Text>
          <Text style={[metin.sm, { fontFamily: 'Inter_600SemiBold', color: c.text, flex: 1 }]} numberOfLines={1}>
            {chosen && code !== null ? (chosen.name ?? 'Elle girilen kod') : suggestionLabel(s)}
          </Text>
          {onEditCode && code !== null ? <EditCodeButton onPress={() => onEditCode(chosen?.code ?? code)} /> : null}
        </View>
        {chosen && code !== null ? (
          <Text style={[metin.xs, { color: c.text2 }]}>Önerilen {proposedText(s.proposed_value)} yerine seçildi; Onayla bu kodu gönderir.</Text>
        ) : (
          <View style={{ flexDirection: 'row', alignItems: 'center', gap: 8 }}>
            <GuvenOlcegi deger={Number(s.confidence)} kaynak={kaynakOf(s)} />
            <Text style={[metin.xs, { color: c.text2, flex: 1 }]} numberOfLines={2}>
              {s.rationale}
            </Text>
          </View>
        )}
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

/**
 * Fişin önerilen hesap kodu (masraf taslağından): Değiştir hesap planı
 * seçicisini açar; Onayla seçilen kodu masrafa yazar.
 */
function ReceiptAccountBox({
  expense,
  chosen,
  onEdit,
}: {
  expense: ReceiptExpense;
  chosen: PickedAccount | null;
  onEdit?: (current: string | null) => void;
}) {
  const { colors: c } = useTheme();
  const pending = expense.approval_status === 'pending';
  const code = chosen?.code ?? expense.account_code ?? null;
  return (
    <View style={[styles.codeBox, { backgroundColor: c.surface2, borderColor: chosen ? c.brand : c.line, gap: 4 }]}>
      <Text style={[metin.xs, { color: c.text3, fontFamily: 'Inter_600SemiBold' }]}>{chosen ? 'SEÇİLEN HESAP KODU' : 'ÖNERİLEN HESAP KODU'}</Text>
      <View style={{ flexDirection: 'row', alignItems: 'center', gap: 8 }}>
        <Text testID="review-account-code" style={[metin.mono, { fontSize: 16, color: c.brandText }]}>
          {code ?? '—'}
        </Text>
        <Text style={[metin.sm, { color: c.text, flex: 1 }]} numberOfLines={1}>
          {chosen ? (chosen.name ?? 'Elle girilen kod') : (expense.category_name ?? '')}
        </Text>
        {onEdit && pending ? <EditCodeButton onPress={() => onEdit(code)} /> : null}
      </View>
      {!pending ? <Text style={[metin.xs, { color: c.text2 }]}>Fiş zaten {expense.approval_status === 'approved' ? 'onaylı' : 'reddedildi'}.</Text> : null}
    </View>
  );
}

/** Kartın mükellef satırı: hangi mükellefin belgesi olduğu her zaman görünür. */
function TaxpayerLine({ name }: { name: string | null }) {
  const { colors: c } = useTheme();
  return (
    <View testID="review-taxpayer" style={{ gap: 1 }}>
      <Text style={[metin.xs, { color: c.text3, fontFamily: 'Inter_600SemiBold' }]}>MÜKELLEF</Text>
      <Text style={[metin.sm, { color: c.text, fontFamily: 'Inter_600SemiBold' }]} numberOfLines={2}>
        {name ?? 'Mükellef yükleniyor…'}
      </Text>
    </View>
  );
}

/** Hesap kodunu değiştir: hesap planı seçicisini açar. */
function EditCodeButton({ onPress }: { onPress: () => void }) {
  const { colors: c } = useTheme();
  return (
    <Pressable
      testID="review-account-edit"
      accessibilityRole="button"
      accessibilityLabel="Hesap kodunu değiştir"
      hitSlop={10}
      onPress={onPress}
      style={({ pressed }) => [styles.editBtn, { borderColor: c.brand, opacity: pressed ? 0.7 : 1 }]}>
      <Text style={[metin.xs, { color: c.brandText, fontFamily: 'Inter_600SemiBold' }]}>Değiştir</Text>
    </Pressable>
  );
}

function TabletDetail({
  s,
  chosen,
  onEditCode,
  allowed,
  busy,
  approving,
  onApprove,
  onReject,
  onCorrect,
}: {
  s: Suggestion;
  chosen: PickedAccount | null;
  onEditCode: (current: string | null) => void;
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
  const taxpayer = useTaxpayerName(s.engagement_uuid);
  const receipt = useOpenReceipt();
  const d = ctx.data;
  const image = d?.receipt && d.receipt.media_type.startsWith('image/') ? d.receipt : null;
  const expense = useReceiptExpense(s.engagement_uuid, d?.receipt?.uuid).data ?? null;
  const code = proposedCode(s.proposed_value);
  return (
    <>
      <View style={[styles.docPane, { backgroundColor: c.surface3 }]}>
        {image ? (
          <Pressable
            testID="review-receipt-thumb"
            accessibilityRole="imagebutton"
            accessibilityLabel="Fiş görselini tam ekran aç"
            onPress={() => receipt.open({ engagement: s.engagement_uuid ?? '', uuid: image.uuid, mediaType: image.media_type, title: taxpayer ?? undefined })}
            style={{ width: '80%', height: '80%' }}>
            <Image source={receiptImageSource(org, s.engagement_uuid ?? '', image.uuid)} style={{ width: '100%', height: '100%' }} contentFit="contain" />
          </Pressable>
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
        <TaxpayerLine name={taxpayer} />
        <View style={{ gap: 4 }}>
          <Text style={[metin.baslik2, { color: c.text }]}>{d?.invoice?.counterparty_name || suggestionLabel(s)}</Text>
          <Text style={[metin.sm, { color: c.text2 }]}>{[periodLabel(s.period ?? null), d?.invoice?.issue_date].filter(Boolean).join(' · ')}</Text>
        </View>
        {d?.item ? (
          <View style={[styles.codeBox, { borderColor: c.line }]}>
            <Row k="Kalem" v={d.item.name || '—'} />
            <Row k="Tutar" v={money(d.item.line_amount, d.invoice?.currency ?? 'TRY')} num />
            <Row k="KDV" v={`%${Number(d.item.tax_rate)}`} num />
          </View>
        ) : null}
        {expense ? <ReceiptAccountBox expense={expense} chosen={chosen} onEdit={allowed ? onEditCode : undefined} /> : null}
        <View style={[styles.codeBox, { borderColor: c.brand }]}>
          <View style={{ flexDirection: 'row', alignItems: 'center', gap: 8 }}>
            <Text testID={code !== null ? 'review-account-code' : undefined} style={[metin.mono, { fontSize: 15, color: c.brandText }]}>
              {(code !== null ? chosen?.code : null) ?? (proposedText(s.proposed_value) || '—')}
            </Text>
            <Text style={[metin.sm, { fontFamily: 'Inter_600SemiBold', color: c.text, flex: 1 }]} numberOfLines={1}>
              {chosen && code !== null ? (chosen.name ?? 'Elle girilen kod') : suggestionLabel(s)}
            </Text>
            {allowed && code !== null ? <EditCodeButton onPress={() => onEditCode(chosen?.code ?? code)} /> : null}
          </View>
          {chosen && code !== null ? (
            <Text style={[metin.xs, { color: c.text2 }]}>Önerilen {proposedText(s.proposed_value)} yerine seçildi; Onayla bu kodu gönderir.</Text>
          ) : null}
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
  thumb: { width: 64, height: 84, borderRadius: 10 },
  editBtn: { borderWidth: 1, borderRadius: 999, paddingHorizontal: 12, minHeight: 30, justifyContent: 'center' },
  hints: { flexDirection: 'row', justifyContent: 'space-between' },
  actions: { flexDirection: 'row', justifyContent: 'center', alignItems: 'center', gap: 18 },
  listPane: { width: 340, borderRightWidth: StyleSheet.hairlineWidth },
  groupHead: { flexDirection: 'row', alignItems: 'center', gap: 8, paddingHorizontal: 20, paddingVertical: 8, borderTopWidth: StyleSheet.hairlineWidth },
  listRow: { flexDirection: 'row', alignItems: 'center', gap: 10, paddingHorizontal: 20, paddingVertical: 12, minHeight: 56, borderTopWidth: StyleSheet.hairlineWidth },
  docPane: { flex: 1, alignItems: 'center', justifyContent: 'center' },
  paper: { width: 300, padding: 24, gap: 8, borderRadius: 4, borderWidth: StyleSheet.hairlineWidth },
  decision: { width: 360, flexGrow: 0, borderLeftWidth: StyleSheet.hairlineWidth },
  bigBtn: { flex: 1, flexDirection: 'row', gap: 8, minHeight: 52, borderRadius: 12, alignItems: 'center', justifyContent: 'center', paddingHorizontal: 16 },
  outline: { borderWidth: 1 },
  bigBtnText: { fontFamily: 'Inter_600SemiBold', fontSize: 16 },
});

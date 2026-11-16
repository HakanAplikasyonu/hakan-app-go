import { useRouter } from 'expo-router';
import { useCallback, useMemo, useState } from 'react';
import { ActivityIndicator, FlatList, Pressable, RefreshControl, StyleSheet, useWindowDimensions, View } from 'react-native';

import { Button, Chip, Notice, Text } from '@/components/ui';
import { errorMessage } from '@/core/api/envelope';
import { useAuth, useOrganization } from '@/core/auth/auth-provider';
import { useColors } from '@/core/theme';
import { decide, useDecide, useInvalidateReview, useReviewQueue, type Suggestion } from '@/features/review/api';
import { bulkTargets, confidenceBand, confidencePercent, fieldLabel, proposedCode, runSequential } from '@/features/review/format';
import { ReasonSheet } from '@/features/review/reason-sheet';
import { SwipeRow } from '@/features/review/swipe-row';
import { periodLabel } from '@/features/warnings/format';

/** İnceleme kuyruğu (#239): sağa kaydır onayla, sola kaydır reddet; uzun bas → toplu seçim. */
export default function ReviewQueueScreen() {
  const c = useColors();
  const router = useRouter();
  const org = useOrganization();
  const { can } = useAuth();
  const allowed = can('tenant.review.decide');
  const { width } = useWindowDimensions();
  const q = useReviewQueue();
  const single = useDecide();
  const refresh = useInvalidateReview();
  const items = useMemo(() => (q.data?.pages.flatMap((p) => p.items) ?? []).filter((s) => s.status === 'pending'), [q.data]);
  const total = q.data?.pages[0]?.total ?? 0;

  const [selected, setSelected] = useState<Set<string>>(new Set());
  const [rejecting, setRejecting] = useState<Suggestion | null>(null);
  const [notice, setNotice] = useState<{ tone: 'info' | 'danger'; text: string } | null>(null);
  const [bulkBusy, setBulkBusy] = useState(false);
  const selecting = selected.size > 0;

  const toggle = useCallback((uuid: string) => {
    setSelected((s) => {
      const next = new Set(s);
      if (next.has(uuid)) next.delete(uuid);
      else next.add(uuid);
      return next;
    });
  }, []);

  const accept = useCallback(
    async (s: Suggestion) => {
      setNotice(null);
      try {
        await single.mutateAsync({ suggestion: s, decision: { action: 'accept' } });
        setNotice({ tone: 'info', text: `${proposedCode(s.proposed_value) ?? 'Öneri'} onaylandı.` });
      } catch (e) {
        setNotice({ tone: 'danger', text: errorMessage(e) });
      }
    },
    [single],
  );

  async function bulkAccept() {
    const targets = bulkTargets(items, selected);
    if (!targets.length) return;
    setBulkBusy(true);
    setNotice(null);
    const res = await runSequential(targets, (s) => decide(org, s, { action: 'accept' }));
    setBulkBusy(false);
    setSelected(new Set(res.failed.map((f) => f.item.uuid)));
    refresh();
    setNotice(
      res.failed.length
        ? { tone: 'danger', text: `${res.ok.length} öneri onaylandı, ${res.failed.length} öneri onaylanamadı: ${errorMessage(res.failed[0].error)}` }
        : { tone: 'info', text: `${res.ok.length} öneri onaylandı.` },
    );
  }

  return (
    <View style={{ flex: 1, backgroundColor: c.background }}>
      {notice ? (
        <View style={{ paddingHorizontal: 16, paddingTop: 12 }}>
          <Notice tone={notice.tone}>{notice.text}</Notice>
        </View>
      ) : null}
      {q.error ? (
        <View style={{ padding: 16 }}>
          <Notice>{errorMessage(q.error)}</Notice>
        </View>
      ) : null}
      <FlatList
        data={items}
        keyExtractor={(s) => s.uuid}
        contentContainerStyle={{ padding: 16, gap: 10, paddingBottom: selecting ? 120 : 24 }}
        refreshControl={<RefreshControl refreshing={q.isRefetching && !q.isFetchingNextPage} onRefresh={() => void q.refetch()} tintColor={c.brand} />}
        onEndReached={() => q.hasNextPage && !q.isFetchingNextPage && q.fetchNextPage()}
        onEndReachedThreshold={0.4}
        ListHeaderComponent={
          q.data ? (
            <Text variant="caption" tone="muted">
              {total} bekleyen öneri{allowed ? ' · sağa kaydır: onayla, sola: reddet, uzun bas: seç' : ' · karar yetkiniz yok, yalnızca görüntüleme'}
            </Text>
          ) : undefined
        }
        ListEmptyComponent={q.isLoading ? <ActivityIndicator color={c.brand} /> : <Text tone="muted">Bekleyen öneri yok.</Text>}
        ListFooterComponent={q.isFetchingNextPage ? <ActivityIndicator color={c.brand} /> : undefined}
        renderItem={({ item: s }) => (
          <SwipeRow
            width={width}
            enabled={allowed && !selecting}
            onAction={(a) => (a === 'accept' ? void accept(s) : setRejecting(s))}>
            <Pressable
              testID="suggestion-row"
              accessibilityRole="button"
              accessibilityState={{ selected: selected.has(s.uuid) }}
              accessibilityLabel={`${fieldLabel(s.target_field)} önerisi ${proposedCode(s.proposed_value) ?? ''}, güven ${confidencePercent(s.confidence)}`}
              accessibilityActions={allowed ? [{ name: 'accept', label: 'Onayla' }, { name: 'reject', label: 'Reddet' }] : undefined}
              onAccessibilityAction={(e) => (e.nativeEvent.actionName === 'accept' ? void accept(s) : setRejecting(s))}
              onLongPress={allowed ? () => toggle(s.uuid) : undefined}
              onPress={() =>
                selecting
                  ? toggle(s.uuid)
                  : router.push({ pathname: '/review/[uuid]', params: { uuid: s.uuid, engagement: s.engagement_uuid ?? '' } })
              }
              style={[styles.card, { backgroundColor: c.surface, borderColor: selected.has(s.uuid) ? c.brand : c.border }]}>
              <View style={{ flexDirection: 'row', alignItems: 'center', gap: 8 }}>
                <Text variant="title" style={{ fontSize: 22 }}>
                  {proposedCode(s.proposed_value) ?? '—'}
                </Text>
                <View style={{ flex: 1 }} />
                <Chip
                  tone={confidenceBand(s.confidence) === 'high' ? 'success' : confidenceBand(s.confidence) === 'medium' ? 'warning' : 'danger'}
                  label={confidencePercent(s.confidence)}
                />
              </View>
              <Text variant="caption" tone="muted">
                {[fieldLabel(s.target_field), periodLabel(s.period ?? null)].filter(Boolean).join(' · ')}
              </Text>
              <Text numberOfLines={2}>{s.rationale}</Text>
            </Pressable>
          </SwipeRow>
        )}
      />
      {selecting ? (
        <View style={[styles.bar, { backgroundColor: c.surface, borderColor: c.border }]}>
          <Text style={{ flex: 1 }}>{selected.size} seçili</Text>
          <Button title="Vazgeç" variant="ghost" onPress={() => setSelected(new Set())} />
          <Button title={`Hepsini onayla`} onPress={() => void bulkAccept()} loading={bulkBusy} />
        </View>
      ) : null}
      <ReasonSheet
        mode={rejecting ? { kind: 'reject' } : null}
        busy={single.isPending}
        error={single.error && rejecting ? errorMessage(single.error) : null}
        onCancel={() => setRejecting(null)}
        onSubmit={async ({ reason }) => {
          if (!rejecting) return;
          try {
            await single.mutateAsync({ suggestion: rejecting, decision: { action: 'reject', reason } });
            setRejecting(null);
            setNotice({ tone: 'info', text: 'Öneri reddedildi.' });
          } catch {
            // hata sayfada gösterilir
          }
        }}
      />
    </View>
  );
}

const styles = StyleSheet.create({
  card: { borderWidth: 1, borderRadius: 16, padding: 14, gap: 4 },
  bar: { position: 'absolute', left: 0, right: 0, bottom: 0, flexDirection: 'row', alignItems: 'center', gap: 8, padding: 16, paddingBottom: 28, borderTopWidth: StyleSheet.hairlineWidth },
});

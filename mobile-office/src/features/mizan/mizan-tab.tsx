import { BosDurum, KristalYukleyici } from '@hakan/mobile-core/hakan';
import { Segment } from '@hakan/mobile-core/ui/parcalar';
import { useMemo, useState } from 'react';
import { Pressable, Share, StyleSheet, Switch, View } from 'react-native';

import { LoadError } from '@/components/load-error';
import { PeriodChips } from '@/components/period-chips';
import { Card, Chip, Field, Notice, Text } from '@/components/ui';
import { errorMessage } from '@/core/api/envelope';
import { useColors } from '@/core/theme';
import { money } from '@/features/clients/format';

import { useTrialBalanceReport, useTrialBalances, type TrialBalanceLine } from './api';
import { balanceLabel, balanceSpoken, changeOf, filterLines, isBalanced, shareText, trialBalanceLabel, type ChangeTone } from './format';

const LEVELS = [
  { key: '1', label: 'Ana hesaplar' },
  { key: '0', label: 'Tüm hesaplar' },
] as const;

/** Uzun mizanda ilk bu kadar satır çizilir; arama daraltır. */
const MAX_ROWS = 300;

/**
 * Mizan sekmesi (#393): son yüklenen mizan (ya da seçilen dönem), borç /
 * alacak toplamları, ana hesap ya da tüm hesap bakiyeleri ve önceki dönemle
 * karşılaştırma. Özet sistem paylaş menüsüyle metin olarak paylaşılır.
 */
export function MizanTab({ engagement, title }: { engagement: string; title?: string }) {
  const c = useColors();
  const list = useTrialBalances(engagement);
  const [picked, setPicked] = useState<string | undefined>();
  const [level, setLevel] = useState<'1' | '0'>('1');
  const [compare, setCompare] = useState(true);
  const [q, setQ] = useState('');

  const items = list.data ?? [];
  const selected = items.find((t) => t.uuid === picked)?.uuid ?? items[0]?.uuid;
  const report = useTrialBalanceReport(engagement, selected, compare, Number(level));
  const data = report.data;
  const lines = useMemo(() => filterLines(data?.lines ?? [], q), [data?.lines, q]);

  if (list.isLoading) {
    return (
      <View style={styles.center}>
        <KristalYukleyici etiket="Mizanlar yükleniyor" />
      </View>
    );
  }
  if (list.error) return <LoadError title="Mizanlar alınamadı" error={list.error} retry={list.refetch} retrying={list.isRefetching} />;
  if (items.length === 0) {
    return (
      <BosDurum
        baslik="Mizan yok"
        aciklama="Bu mükellefe tutarlı bir mizan yüklenmedi. Mizan ofis panelinden (Hesap planı → içe aktar) Luca, Zirve ya da Logo mizan dökümüyle yüklenir."
      />
    );
  }

  const tb = data?.trial_balance;
  const label = tb ? trialBalanceLabel(tb) : '';
  const comparedLabel = data?.compared_to ? trialBalanceLabel(data.compared_to) : null;

  async function share() {
    if (!data || !tb) return;
    await Share.share({ title: `${label} mizanı`, message: shareText(title ?? 'Mükellef', label, tb, data.lines, comparedLabel) });
  }

  return (
    <View style={{ gap: 14 }} testID="mizan-tab">
      <PeriodChips
        periods={items.map((t) => ({ key: t.uuid, label: trialBalanceLabel(t) }))}
        selected={selected}
        onSelect={setPicked}
        label={(p) => `${p.label} mizanı`}
        testIDPrefix="mizan-period"
      />

      {report.error ? <Notice>{errorMessage(report.error)}</Notice> : null}

      {tb ? (
        <Card>
          <View style={styles.headRow}>
            <Text variant="heading" style={{ flex: 1 }}>
              {label} mizanı
            </Text>
            <Pressable
              accessibilityRole="button"
              accessibilityLabel={`${label} mizan özetini paylaş`}
              hitSlop={8}
              onPress={() => void share()}
              testID="mizan-share">
              <Text tone="brand" style={{ fontWeight: '600' }}>
                Paylaş
              </Text>
            </Pressable>
          </View>
          <Text variant="caption" tone="muted" numberOfLines={1}>
            {tb.file_name} · {tb.account_count} hesap
          </Text>
          <Stat label="Borç toplamı" value={money(tb.debit_total)} />
          <Stat label="Alacak toplamı" value={money(tb.credit_total)} />
          <View style={{ flexDirection: 'row' }}>
            {isBalanced(tb) ? <Chip tone="success" label="Mizan dengeli" /> : <Chip tone="danger" label="Borç ve alacak tutmuyor" />}
          </View>
        </Card>
      ) : null}

      <Segment secenekler={LEVELS} secili={level} onSec={setLevel} testIDOnek="mizan-level" />
      <View style={styles.headRow}>
        <Text style={{ flex: 1 }} nativeID="mizan-compare-label">
          {comparedLabel && compare ? `${comparedLabel} ile karşılaştır` : 'Önceki dönemle karşılaştır'}
        </Text>
        <Switch
          testID="mizan-compare"
          accessibilityLabel="Önceki dönemle karşılaştır"
          accessibilityLabelledBy="mizan-compare-label"
          value={compare}
          onValueChange={setCompare}
          trackColor={{ true: c.brand, false: c.border }}
        />
      </View>
      {compare && data && !data.compared_to ? <Text tone="muted">Karşılaştırılacak önceki dönem mizanı yok.</Text> : null}
      <Field label="Hesap ara" placeholder="Kod ya da hesap adı" value={q} onChangeText={setQ} autoCorrect={false} testID="mizan-search" />

      {report.isLoading ? (
        <View style={styles.center}>
          <KristalYukleyici etiket="Mizan yükleniyor" />
        </View>
      ) : null}
      {data && lines.length === 0 ? <Text tone="muted">Aramayla eşleşen hesap yok.</Text> : null}
      <View style={{ gap: 8, opacity: report.isFetching && !report.isLoading ? 0.6 : 1 }}>
        {lines.slice(0, MAX_ROWS).map((l) => (
          <LineRow key={l.code} line={l} />
        ))}
      </View>
      {lines.length > MAX_ROWS ? (
        <Text tone="muted" variant="caption">
          İlk {MAX_ROWS} hesap gösteriliyor ({lines.length} hesap). Aramayla daraltın.
        </Text>
      ) : null}
    </View>
  );
}

function LineRow({ line }: { line: TrialBalanceLine }) {
  const c = useColors();
  const ch = changeOf(line);
  const tone: Record<ChangeTone, string> = { up: c.success, down: c.danger, flat: c.muted, new: c.brand };
  const spoken = [`${line.code} ${line.name}`, balanceSpoken(line.balance), ch ? `değişim ${ch.text}${ch.percent ? ` ${ch.percent}` : ''}` : null]
    .filter(Boolean)
    .join(', ');
  return (
    <View
      testID="mizan-line"
      accessible
      accessibilityLabel={spoken}
      style={[styles.line, { backgroundColor: c.surface, borderColor: c.border, marginLeft: (line.level - 1) * 12 }]}>
      <View style={{ flex: 1, gap: 2 }}>
        <Text style={{ fontWeight: line.level === 1 ? '700' : '500' }} numberOfLines={2}>
          {line.code} {line.name}
        </Text>
        {ch ? (
          <Text variant="caption" style={{ color: tone[ch.tone] }}>
            {ch.text}
            {ch.percent ? ` (${ch.percent})` : ''}
          </Text>
        ) : null}
      </View>
      <Text style={{ fontWeight: '600' }}>{balanceLabel(line.balance)}</Text>
    </View>
  );
}

function Stat({ label, value }: { label: string; value: string }) {
  return (
    <View style={styles.headRow} accessible accessibilityLabel={`${label}: ${value}`}>
      <Text tone="muted" style={{ flex: 1 }}>
        {label}
      </Text>
      <Text style={{ fontWeight: '600' }}>{value}</Text>
    </View>
  );
}

const styles = StyleSheet.create({
  center: { alignItems: 'center', paddingVertical: 40 },
  headRow: { flexDirection: 'row', alignItems: 'center', gap: 8, minHeight: 24 },
  line: { flexDirection: 'row', alignItems: 'center', gap: 10, borderWidth: StyleSheet.hairlineWidth, borderRadius: 12, padding: 12 },
});

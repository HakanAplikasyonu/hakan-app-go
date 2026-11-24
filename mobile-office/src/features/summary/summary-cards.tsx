import { ActivityIndicator, StyleSheet, View } from 'react-native';

import { Card, Chip, Text } from '@/components/ui';
import { useAuth } from '@/core/auth/auth-provider';
import { useColors } from '@/core/theme';
import { periodLabel } from '@/features/warnings/format';

import { useIntegrationIssues, useMissingDocuments, useVatForecast, useZReportDates } from './api';
import { cents, directionTotals, gapLines, istanbulToday, missingPeriod, tl, vatBreakdown, vatResult, zGapDays } from './format';

/**
 * Dönemin KDV hesabı (Oğuz'un canlı geri bildirimi): ödenecek ya da
 * devreden KDV, oran oran hesaplanan / indirilecek KDV ve gelen / giden
 * fatura toplamları. Kaynak: KDV tahmin ucu (faturalar; web'deki KDV
 * tahminiyle aynı hesap).
 */
export function VatSummaryCard({ engagement, period }: { engagement: string; period: string }) {
  const { can } = useAuth();
  const c = useColors();
  const allowed = can('tenant.vat.read');
  const q = useVatForecast(engagement, period, allowed);
  if (!allowed) return null;
  const f = q.data;
  const result = vatResult(f?.totals);
  const rows = vatBreakdown(f?.rates);
  const sides = directionTotals(f?.rates);
  const carriedIn = cents(f?.totals.carried_in);

  return (
    <View testID="vat-summary">
      <Card>
        <Text variant="heading">{periodLabel(period)} KDV hesabı</Text>
        {q.isLoading ? <ActivityIndicator color={c.brand} /> : null}
        {q.error && !f ? (
          <Text tone="danger" variant="caption">
            KDV hesabı alınamadı; sayfayı aşağı çekip yeniden deneyin.
          </Text>
        ) : null}
        {result ? (
          <View style={{ gap: 2 }}>
            <Text variant="caption" tone="muted">
              {result.title}
            </Text>
            <Text
              variant="title"
              testID="vat-result"
              style={{ color: result.kind === 'payable' ? c.danger : result.kind === 'carry' ? c.success : c.text }}>
              {tl(result.amount)}
            </Text>
            <Text variant="caption" tone="muted">
              {result.text}
              {carriedIn ? ` · önceki aydan devreden ${tl(carriedIn)} düşüldü` : ''}
            </Text>
          </View>
        ) : null}
        {f ? (
          <>
            <View style={styles.sides}>
              <Side title="Giden faturalar" base={sides.outgoing.base} vat={sides.outgoing.vat} total={sides.outgoing.total} testID="vat-outgoing" />
              <Side title="Gelen faturalar" base={sides.incoming.base} vat={sides.incoming.vat} total={sides.incoming.total} testID="vat-incoming" />
            </View>
            {rows.length ? (
              <View style={{ gap: 6 }}>
                <Text variant="caption" tone="muted" style={{ fontWeight: '600' }}>
                  Oran kırılımı
                </Text>
                {rows.map((r) => (
                  <View key={r.rate} style={styles.rateRow} testID="vat-rate-row">
                    <Text style={{ fontWeight: '600', width: 70 }}>{r.label}</Text>
                    <View style={{ flex: 1, gap: 1 }}>
                      <Text variant="caption">Hesaplanan {tl(r.outputVat)}</Text>
                      <Text variant="caption">İndirilecek {tl(r.inputVat)}</Text>
                    </View>
                    <Text variant="caption" style={{ fontWeight: '600', color: r.net > 0 ? c.danger : r.net < 0 ? c.success : c.muted }}>
                      {r.net > 0 ? '+' : ''}
                      {tl(r.net)}
                    </Text>
                  </View>
                ))}
              </View>
            ) : (
              <Text tone="muted">Bu ay KDV içeren fatura yok.</Text>
            )}
            <Text variant="caption" tone="muted">
              Faturalardan hesaplanan tahmindir; beyanname hazırlanınca kesinleşir.
            </Text>
          </>
        ) : null}
      </Card>
    </View>
  );
}

function Side({ title, base, vat, total, testID }: { title: string; base: number; vat: number; total: number; testID: string }) {
  const c = useColors();
  return (
    <View testID={testID} style={[styles.side, { borderColor: c.border }]}>
      <Text variant="caption" tone="muted" style={{ fontWeight: '600' }}>
        {title}
      </Text>
      <Text style={{ fontWeight: '700' }}>{tl(total)}</Text>
      <Text variant="caption" tone="muted">
        Matrah {tl(base)}
      </Text>
      <Text variant="caption" tone="muted">
        KDV {tl(vat)}
      </Text>
    </View>
  );
}

/**
 * "Eksik / gedik" kartı (Oğuz'un canlı geri bildirimi): kapanmış ayın eksik
 * banka ekstreleri ve gelmeyen belgeleri, seçili ayda Z raporu olmayan
 * günler (mükellef Z raporu kullanıyorsa), eşitleme hatası olan bağlantılar.
 * Hiç eksik yoksa bunu da söyler.
 */
export function GapCard({ engagement, period, current }: { engagement: string; period: string; current: string }) {
  const { can } = useAuth();
  const c = useColors();
  const allowed = { docs: can('tenant.doc_tasks.read'), receipts: can('tenant.receipts.read'), invoices: can('tenant.invoices.read'), bank: can('tenant.bank.read') };
  const mp = missingPeriod(period, current);
  const missing = useMissingDocuments(engagement, mp, allowed.docs);
  const z = useZReportDates(engagement, period, allowed.receipts);
  const integrations = useIntegrationIssues(engagement, { invoices: allowed.invoices, bank: allowed.bank });
  if (!allowed.docs && !allowed.receipts && !allowed.invoices && !allowed.bank) return null;

  const zDays = z.data && z.data.total > 0 ? zGapDays(period, z.data.dates, istanbulToday()) : [];
  const lines = gapLines({
    missing: missing.data ? { period: missing.data.period, items: missing.data.items } : null,
    zGaps: { period, days: zDays },
    integrations: integrations.issues,
  });
  const loading = missing.isLoading || z.isLoading || integrations.loading;

  return (
    <View testID="gap-card">
      <Card>
        <View style={{ flexDirection: 'row', alignItems: 'center', gap: 8 }}>
          <Text variant="heading" style={{ flex: 1 }}>
            Eksik / gedik
          </Text>
          {lines.length ? <Chip tone="warning" label={String(lines.length)} /> : null}
        </View>
        {loading ? <ActivityIndicator color={c.brand} /> : null}
        {!loading && !lines.length ? <Text tone="muted">Eksik belge, Z raporu boşluğu ya da eşitleme hatası görünmüyor.</Text> : null}
        {lines.map((l) => (
          <View key={l.key} style={styles.gap} testID="gap-line">
            <View style={[styles.dot, { backgroundColor: l.tone === 'danger' ? c.danger : c.warning }]} />
            <View style={{ flex: 1, gap: 1 }}>
              <Text style={{ fontWeight: '600' }}>{l.title}</Text>
              <Text variant="caption" tone="muted">
                {l.detail}
              </Text>
            </View>
          </View>
        ))}
        {missing.error && !missing.data ? (
          <Text variant="caption" tone="muted">
            Eksik belge listesi alınamadı.
          </Text>
        ) : null}
      </Card>
    </View>
  );
}

const styles = StyleSheet.create({
  sides: { flexDirection: 'row', gap: 10 },
  side: { flex: 1, borderWidth: StyleSheet.hairlineWidth, borderRadius: 12, padding: 10, gap: 2 },
  rateRow: { flexDirection: 'row', alignItems: 'center', gap: 8 },
  gap: { flexDirection: 'row', alignItems: 'flex-start', gap: 10 },
  dot: { width: 8, height: 8, borderRadius: 4, marginTop: 7 },
});

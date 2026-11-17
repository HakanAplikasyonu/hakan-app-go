import { BosDurum, KristalYukleyici } from '@hakan/mobile-core/hakan';
import { useMemo, useState } from 'react';
import { StyleSheet, View } from 'react-native';

import { PeriodChips } from '@/components/period-chips';
import { Card, Chip, Notice, Text } from '@/components/ui';
import { errorMessage } from '@/core/api/envelope';
import { money } from '@/features/clients/format';
import { FileRow } from '@/features/files/file-row';
import { periodLabel } from '@/features/warnings/format';

import { payrollFile, usePayrollDocuments, usePayrollRuns } from './api';
import { groupByEmployee, kindLabels, paymentLabels, periodsOf } from './format';

const SGK: Record<'accrued' | 'partial' | 'paid' | 'overdue', { label: string; tone: 'warning' | 'success' | 'danger' }> = {
  accrued: { label: 'SGK tahakkuk edildi', tone: 'warning' },
  partial: { label: 'SGK kısmen ödendi', tone: 'warning' },
  paid: { label: 'SGK ödendi', tone: 'success' },
  overdue: { label: 'SGK ödemesi gecikti', tone: 'danger' },
};

/**
 * Bordro sekmesi (#393): dönem seçici, dönem toplamları ve ödeme durumu,
 * bordro PDF'leri personel bazında (önizleme + paylaş). Salt okuma.
 */
export function PayrollTab({ engagement }: { engagement: string }) {
  const runs = usePayrollRuns(engagement);
  const docs = usePayrollDocuments(engagement, undefined);
  const [picked, setPicked] = useState<string | undefined>();
  const [fileError, setFileError] = useState<string | null>(null);

  const periods = useMemo(() => periodsOf(runs.data ?? [], docs.data ?? []), [runs.data, docs.data]);
  const period = picked && periods.includes(picked) ? picked : periods[0];
  const run = runs.data?.find((r) => r.period === period);
  const groups = useMemo(() => groupByEmployee((docs.data ?? []).filter((d) => d.period === period)), [docs.data, period]);

  if (runs.isLoading || docs.isLoading) {
    return (
      <View style={styles.center}>
        <KristalYukleyici etiket="Bordrolar yükleniyor" />
      </View>
    );
  }
  const error = runs.error ?? docs.error;
  if (error) return <Notice>{errorMessage(error)}</Notice>;
  if (periods.length === 0) {
    return <BosDurum baslik="Bordro yok" aciklama="Bu mükellefe henüz bordro yüklenmedi. Bordrolar ofis panelinden içe aktarılır." />;
  }

  return (
    <View style={{ gap: 14 }} testID="payroll-tab">
      <PeriodChips
        periods={periods.map((p) => ({ key: p, label: periodLabel(p) ?? p }))}
        selected={period}
        onSelect={setPicked}
        label={(p) => `${p.label} bordrosu`}
        testIDPrefix="payroll-period"
      />
      {fileError ? <Notice>{fileError}</Notice> : null}

      {run ? (
        <Card>
          <Text variant="heading">{periodLabel(run.period)} bordrosu</Text>
          <Stat label="Personel" value={String(run.totals.employee_count)} />
          <Stat label="Brüt toplam" value={money(run.totals.gross_total)} />
          <Stat label="Net toplam" value={money(run.totals.net_total)} />
          <Stat label="İşveren maliyeti" value={money(run.totals.employer_cost_total)} />
          <View style={styles.chips}>
            <Chip tone={paymentLabels[run.payment_status].tone} label={paymentLabels[run.payment_status].label} />
            {run.sgk ? <Chip tone={SGK[run.sgk.status].tone} label={SGK[run.sgk.status].label} /> : null}
          </View>
        </Card>
      ) : null}

      {groups.length === 0 ? <Text tone="muted">Bu dönem için bordro PDF’i yok.</Text> : null}
      {groups.map((g) => (
        <View key={g.key} style={{ gap: 8 }}>
          <Text variant="heading" accessibilityRole="header">
            {g.title}
          </Text>
          {g.items.map((d) => (
            <FileRow
              key={d.uuid}
              testID="payroll-document"
              file={payrollFile(engagement, d)}
              subtitle={kindLabels[d.kind]}
              onError={setFileError}
            />
          ))}
        </View>
      ))}
    </View>
  );
}

function Stat({ label, value }: { label: string; value: string }) {
  return (
    <View style={styles.stat} accessible accessibilityLabel={`${label}: ${value}`}>
      <Text tone="muted" style={{ flex: 1 }}>
        {label}
      </Text>
      <Text style={{ fontWeight: '600' }}>{value}</Text>
    </View>
  );
}

const styles = StyleSheet.create({
  center: { alignItems: 'center', paddingVertical: 40 },
  chips: { flexDirection: 'row', flexWrap: 'wrap', gap: 6, paddingTop: 4 },
  stat: { flexDirection: 'row', alignItems: 'center', gap: 8, minHeight: 24 },
});

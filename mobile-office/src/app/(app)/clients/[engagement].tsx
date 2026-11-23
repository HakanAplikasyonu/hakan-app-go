import { Image } from 'expo-image';
import { Stack, useLocalSearchParams, useRouter } from 'expo-router';
import { useMemo, useState } from 'react';
import { ActivityIndicator, Linking, Pressable, StyleSheet, View } from 'react-native';

import { Button, Card, Chip, Notice, Row, Screen, Text } from '@/components/ui';
import { errorMessage } from '@/core/api/envelope';
import { useAuth, useOrganization } from '@/core/auth/auth-provider';
import { useColors } from '@/core/theme';
import {
  receiptImageSource,
  useContacts,
  useOpenTasks,
  usePeriod,
  useRecentInvoices,
  useRecentReceipts,
} from '@/features/clients/api';
import { mailHref, money, recentPeriods, telHref } from '@/features/clients/format';
import { useArchiveCounts } from '@/features/archive/api';
import { NextWorkCard } from '@/features/copilot/next-work';
import { expiryWarning } from '@/features/archive/format';
import { ForeignInvoicesCard } from '@/features/foreign-invoices/foreign-invoices-card';
import { MonthCards } from '@/features/month/month-cards';
import { RecordTabs } from '@/features/month/record-tabs';
import { useOpenReceipt } from '@/features/files/receipt';
import { GapCard, VatSummaryCard } from '@/features/summary/summary-cards';
import { visibleTabs } from '@/features/documents/tabs';
import { canRemind } from '@/features/doctasks/channels';
import { statusLabel, statusTone, taskTitle, type TaskStatus } from '@/features/doctasks/format';
import { RemindButton } from '@/features/doctasks/remind-button';
import { periodLabel } from '@/features/warnings/format';

/** Mükellef ayrıntısı (#238): dönem kartları (#780), iletişim, dönem durumu, son belgeler, açık görevler. */
export default function ClientDetailScreen() {
  const { engagement, title } = useLocalSearchParams<{ engagement: string; title?: string }>();
  const { can } = useAuth();
  const router = useRouter();
  const org = useOrganization();
  const c = useColors();
  const [current, previous] = useMemo(() => recentPeriods(), []);
  // Dört kartın dönemi (#780): bu ay ya da geçmiş bir ay.
  const [monthPeriod, setMonthPeriod] = useState(current);
  const receipt = useOpenReceipt();

  const contacts = useContacts(engagement);
  const cur = usePeriod(engagement, current, can('tenant.periods.read'));
  const prev = usePeriod(engagement, previous, can('tenant.periods.read'));
  const invoices = useRecentInvoices(engagement, can('tenant.invoices.read'));
  const receipts = useRecentReceipts(engagement, can('tenant.receipts.read'));
  const tasks = useOpenTasks(engagement, can('tenant.doc_tasks.read'));

  const docTabs = visibleTabs(can).tabs;
  const archiveCounts = useArchiveCounts(engagement, can('tenant.documents.read'));
  const ac = archiveCounts.data;
  const isgWarning = ac ? expiryWarning(ac.isg_expired, ac.isg_expiring, ac.expiringDays) : null;

  const ct = contacts.data;
  const phone = telHref(ct?.phone);
  const emails = [ct?.email, ...(ct?.members.map((m) => m.email) ?? [])].filter((e, i, a): e is string => !!e && a.indexOf(e) === i);

  return (
    <Screen>
      <Stack.Screen options={{ title: ct?.title ?? title ?? 'Mükellef' }} />
      {contacts.error ? <Notice>{errorMessage(contacts.error)}</Notice> : null}

      <View style={{ gap: 4 }}>
        <Text variant="title" numberOfLines={3}>
          {ct?.title ?? title}
        </Text>
        {ct ? (
          <Text tone="muted">
            {ct.kind === 'legal' ? 'VKN' : 'TCKN'} {ct.tax_id}
            {ct.tax_office ? ` · ${ct.tax_office}` : ''}
          </Text>
        ) : null}
      </View>

      {/* #780 (TestFlight): dönemin Fatura, Z raporu, Masraf ve Banka kartları; karta dokununca o ayın listesi. */}
      <MonthCards engagement={engagement} title={ct?.title ?? title ?? ''} period={monthPeriod} onPeriod={setMonthPeriod} />

      {/* Oğuz'un canlı geri bildirimi: seçili ayın KDV hesabı (oran kırılımı, gelen / giden toplamları) ve eksikler. */}
      <VatSummaryCard engagement={engagement} period={monthPeriod} />
      <GapCard engagement={engagement} period={monthPeriod} current={current} />

      <Card>
        <Text variant="heading">İletişim</Text>
        {contacts.isLoading ? <ActivityIndicator color={c.brand} /> : null}
        {phone ? <ActionLink label={`Ara: ${ct?.phone}`} href={phone} /> : null}
        {emails.map((e) => {
          const href = mailHref(e);
          return href ? <ActionLink key={e} label={`E-posta: ${e}`} href={href} /> : null;
        })}
        {ct && !phone && emails.length === 0 ? <Text tone="muted">Kayıtlı iletişim bilgisi yok.</Text> : null}
        {ct?.members.length ? (
          <Text variant="caption" tone="muted">
            Hesaplar: {ct.members.map((m) => `${m.name} ${m.surname} (${m.role === 'owner' ? 'sahip' : 'çalışan'})`).join(', ')}
          </Text>
        ) : null}
      </Card>

      {can('tenant.periods.read') ? (
        <Card>
          <Text variant="heading">Dönem durumu</Text>
          {[cur, prev].map((p, i) => {
            const period = i === 0 ? current : previous;
            const d = p.data;
            return (
              <View key={period} style={styles.periodRow}>
                <Text style={{ flex: 1 }}>{periodLabel(period)}</Text>
                {p.isLoading ? <ActivityIndicator color={c.brand} /> : null}
                {d ? (
                  <View style={{ flexDirection: 'row', gap: 6 }}>
                    {d.locked ? <Chip label="Kilitli" /> : null}
                    <Chip tone={d.ready ? 'success' : 'danger'} label={d.ready ? 'Hazır' : `${d.open_blockers} engelleyici`} />
                  </View>
                ) : null}
              </View>
            );
          })}
        </Card>
      ) : null}

      {can('tenant.doc_tasks.read') ? (
        <View style={{ flexDirection: 'row', gap: 10 }}>
          {can('tenant.doc_tasks.write') ? (
            <View style={{ flex: 1 }}>
              <Button title="Belge iste" onPress={() => router.push({ pathname: '/tasks/new', params: { engagement, title: ct?.title ?? title ?? '' } })} />
            </View>
          ) : null}
          <View style={{ flex: 1 }}>
            <Button
              title="Görevler"
              variant="secondary"
              onPress={() => router.push({ pathname: '/tasks', params: { engagement, title: ct?.title ?? title ?? '' } })}
            />
          </View>
        </View>
      ) : null}

      {/* Co-pilot (#392): dönem kapanışında sıradaki iş. "Belge iste"nin altında:
          task-create akışı düğmeyi kaydırmadan görür. */}
      {can('tenant.engagements.read') ? <NextWorkCard engagement={engagement} title={ct?.title ?? title ?? ''} /> : null}

      {/* #780: yabancı faturalar / KDV2; ofis ayarı kapalıysa kart kendini gizler. */}
      <ForeignInvoicesCard engagement={engagement} />

      {docTabs.length ? (
        <Card>
          <Text variant="heading">Belgeler</Text>
          <View style={{ gap: 8 }}>
            {docTabs.map((t) => (
              <View key={t.key} testID={`client-documents-${t.key}`}>
                <Row
                  title={t.label}
                  subtitle={t.key === 'isg' && isgWarning ? isgWarning : undefined}
                  trailing={
                    t.key === 'isg' && ac && ac.isg_expired + ac.isg_expiring > 0 ? (
                      <Chip tone={ac.isg_expired > 0 ? 'danger' : 'warning'} label={String(ac.isg_expired + ac.isg_expiring)} />
                    ) : undefined
                  }
                  accessibilityLabel={`${t.label} belgelerini aç${t.key === 'isg' && isgWarning ? `. ${isgWarning}` : ''}`}
                  onPress={() => router.push({ pathname: '/belgeler/[engagement]', params: { engagement, title: ct?.title ?? title ?? '', tab: t.key } })}
                />
              </View>
            ))}
          </View>
        </Card>
      ) : null}

      {/* Kayıt sekmeleri (Oğuz'un canlı geri bildirimi): gelen / giden faturalar, banka, Z raporları, masraflar; satır belge ayrıntısını açar.
          Belgeler kartının altında: mevcut akışlar kaydırmadan aynı yere ulaşır. */}
      <RecordTabs engagement={engagement} title={ct?.title ?? title ?? ''} period={monthPeriod} onPeriod={setMonthPeriod} />

      {tasks.data ? (
        <Section title={`Açık istekler (${tasks.data.total})`} empty={tasks.data.items.length === 0 ? 'Açık belge isteği yok.' : null}>
          {tasks.data.items.map((t) => (
            <View key={t.uuid} style={{ gap: 6 }}>
              <Row
                onPress={() => router.push({ pathname: '/tasks/[uuid]', params: { uuid: t.uuid, engagement } })}
                title={taskTitle(t)}
                subtitle={`${periodLabel(t.period)} · son gün ${new Date(t.due_at).toLocaleDateString('tr-TR')}${t.overdue ? ' · gecikti' : ''}`}
                trailing={
                  <Chip
                    tone={t.overdue ? 'danger' : statusTone(t.status as TaskStatus)}
                    label={t.overdue ? 'Gecikti' : (statusLabel[t.status as TaskStatus] ?? t.status)}
                  />
                }
              />
              {can('tenant.doc_tasks.write') && canRemind(t.status) ? <RemindButton engagement={engagement} task={t.uuid} title={taskTitle(t)} /> : null}
            </View>
          ))}
        </Section>
      ) : null}

      {receipts.data ? (
        <Section title="Son fişler" empty={receipts.data.items.length === 0 ? 'Fiş yok.' : null}>
          <View style={styles.thumbs}>
            {receipts.data.items.map((r) => (
              <Pressable
                key={r.uuid}
                testID="recent-receipt"
                accessibilityRole="button"
                accessibilityLabel={`${new Date(r.created_at).toLocaleDateString('tr-TR')} tarihli fişi ${r.media_type.startsWith('image/') ? 'tam ekran aç' : 'aç'}`}
                onPress={() => receipt.open({ engagement, uuid: r.uuid, mediaType: r.media_type, title: ct?.title ?? title ?? undefined })}
                style={({ pressed }) => ({ gap: 4, width: 96, opacity: pressed ? 0.8 : 1 })}>
                {r.media_type.startsWith('image/') ? (
                  <Image
                    source={receiptImageSource(org, engagement, r.uuid)}
                    style={[styles.thumb, { backgroundColor: c.border }]}
                    contentFit="cover"
                    transition={150}
                    accessibilityLabel="Fiş görseli"
                  />
                ) : (
                  <View style={[styles.thumb, { backgroundColor: c.border, alignItems: 'center', justifyContent: 'center' }]}>
                    {receipt.busy === r.uuid ? <ActivityIndicator color={c.brand} /> : <Text variant="caption">PDF</Text>}
                  </View>
                )}
                <Text variant="caption" tone="muted" numberOfLines={1}>
                  {new Date(r.created_at).toLocaleDateString('tr-TR')}
                </Text>
              </Pressable>
            ))}
          </View>
          {receipt.error ? <Notice>{receipt.error}</Notice> : null}
        </Section>
      ) : null}

      {invoices.data ? (
        <Section title="Son faturalar" empty={invoices.data.items.length === 0 ? 'Fatura yok.' : null}>
          {invoices.data.items.map((inv) => (
            <View key={inv.uuid} testID="recent-invoice">
              <Row
                title={inv.counterparty_name || inv.invoice_number}
                subtitle={`${inv.direction === 'incoming' ? 'Gelen' : 'Giden'} · ${inv.issue_date} · ${money(inv.payable_amount, inv.currency)}`}
                trailing={inv.open_warnings ? <Chip tone="warning" label={`${inv.open_warnings}`} /> : undefined}
                accessibilityLabel={`${inv.counterparty_name || inv.invoice_number} faturasının ayrıntısını aç`}
                onPress={() =>
                  router.push({
                    pathname: '/belge/[uuid]',
                    params: { uuid: inv.uuid, engagement, kind: 'invoices', period: inv.period, title: ct?.title ?? title ?? '' },
                  })
                }
              />
            </View>
          ))}
        </Section>
      ) : null}
    </Screen>
  );
}

function Section({ title, empty, children }: { title: string; empty: string | null; children: React.ReactNode }) {
  return (
    <View style={{ gap: 8 }}>
      <Text variant="heading">{title}</Text>
      {empty ? <Text tone="muted">{empty}</Text> : children}
    </View>
  );
}

function ActionLink({ label, href }: { label: string; href: string }) {
  return (
    <Pressable accessibilityRole="link" onPress={() => void Linking.openURL(href)} hitSlop={8}>
      <Text tone="brand" style={{ fontWeight: '600' }}>
        {label}
      </Text>
    </Pressable>
  );
}

const styles = StyleSheet.create({
  periodRow: { flexDirection: 'row', alignItems: 'center', gap: 8, minHeight: 32 },
  thumbs: { flexDirection: 'row', flexWrap: 'wrap', gap: 10 },
  thumb: { width: 96, height: 128, borderRadius: 10 },
});

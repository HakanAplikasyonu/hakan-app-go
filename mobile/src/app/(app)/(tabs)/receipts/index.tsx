import { BosDurum, KristalYukleyici, TacRozeti } from '@hakan/mobile-core/hakan';
import { EkranBasligi, metin, Satir, Segment } from '@hakan/mobile-core/ui/parcalar';
import { useTheme } from '@hakan/mobile-core/ui/theme';
import { router } from 'expo-router';
import { useMemo, useState } from 'react';
import { RefreshControl, SectionList, StyleSheet, Text, View } from 'react-native';
import { SafeAreaView } from 'react-native-safe-area-context';

import { Button } from '@/components/ui';
import { useAuth } from '@/core/auth/auth-provider';
import { belgeyiAc, type AcilacakBelge } from '@/features/documents/ac';
import { faturaBelgesi, fisBelgesi } from '@/features/documents/api';
import { taskCrown } from '@/features/home/crown';
import { ayGruplari, sunucuSatiri, tl, yerelSatir, type GecmisSatiri } from '@/features/receipts/gecmis';
import { serverOnly } from '@/features/receipts/history';
import { useReceipts } from '@/features/receipts/provider';
import { receiptItems } from '@/features/receipts/queue';
import { useInvoiceHistory, useReceiptHistory } from '@/features/receipts/use-history';
import { taskStatusLabel, taskTitle } from '@/features/tasks/tasks';
import { useTasks } from '@/features/tasks/use-tasks';

type Sekme = 'fis' | 'fatura' | 'istek';

const sekmeler: { key: Sekme; label: string }[] = [
  { key: 'fis', label: 'Fişler' },
  { key: 'fatura', label: 'Faturalar' },
  { key: 'istek', label: 'İstekler' },
];

/** Geçmiş (MukellefMobil · 5): Fiş / Fatura / İstek sekmeleri; aya göre gruplu, kuyruk etiketleri renkli. */
export default function HistoryScreen() {
  const { colors: c } = useTheme();
  const { taxpayer } = useAuth();
  const [sekme, setSekme] = useState<Sekme>('fis');
  const { items: queue, syncing, syncNow } = useReceipts();
  const { history, loading, refresh } = useReceiptHistory();
  const invoices = useInvoiceHistory(sekme === 'fatura');
  const { query: tasksQ } = useTasks();
  const tp = taxpayer?.taxpayer.uuid ?? '';

  const sections = useMemo(() => {
    if (sekme === 'fis') {
      const local = receiptItems(queue).filter((i) => i.taxpayer === tp);
      const rows = [...local.map(yerelSatir), ...serverOnly(history, local, tp).map(sunucuSatiri)];
      return ayGruplari(rows);
    }
    if (sekme === 'fatura') {
      const rows: GecmisSatiri[] = (invoices.data ?? []).map((f) => ({
        key: f.uuid,
        baslik: f.counterparty_name || f.invoice_number,
        alt: `${f.direction === 'incoming' ? 'Gelen' : 'Giden'} · ${f.invoice_number}${f.status === 'cancelled' ? ' · iptal' : ''}`,
        ton: f.status === 'cancelled' ? 'danger' : null,
        tutar: Number(f.payable_amount) || null,
        tac: f.status === 'cancelled' ? 'sorun' : 'yuklendi',
        zaman: Date.parse(`${f.issue_date}T12:00:00Z`),
        yerelId: null,
      }));
      return ayGruplari(rows).map((g) => ({ ...g, baslik: g.baslik.replace(/ fiş\b/, ' fatura') }));
    }
    const rows: GecmisSatiri[] = (tasksQ.data ?? []).map((t) => ({
      key: t.uuid,
      baslik: taskTitle(t),
      alt: taskStatusLabel(t.status),
      ton: t.overdue && (t.status === 'open' || t.status === 'submitted') ? 'danger' : null,
      tutar: null,
      tac: taskCrown(t.status, t.overdue),
      zaman: Date.parse(t.created_at),
      yerelId: `task:${t.uuid}`,
    }));
    return ayGruplari(rows).map((g) => ({ ...g, baslik: g.baslik.replace(/ fiş\b/, ' istek') }));
  }, [sekme, queue, history, tp, invoices.data, tasksQ.data]);

  // Sunucudaki fişin fotoğrafı ve fatura PDF'i görüntüleyicide açılır (#780).
  const belgeler = useMemo(() => {
    const m = new Map<string, AcilacakBelge>();
    for (const h of history) {
      const b = fisBelgesi(h);
      if (b) m.set(`s:${h.uuid}`, b);
    }
    for (const f of invoices.data ?? []) m.set(f.uuid, faturaBelgesi(f));
    return m;
  }, [history, invoices.data]);

  const busy = sekme === 'fis' ? loading && !history.length : sekme === 'fatura' ? invoices.isLoading : tasksQ.isLoading;
  const tone = (t: GecmisSatiri['ton']) => (t === 'danger' ? c.danger : t === 'warn' ? c.warn : undefined);

  return (
    <SafeAreaView style={{ flex: 1, backgroundColor: c.bg }} edges={['top', 'left', 'right']}>
      <EkranBasligi baslik="Geçmiş" />
      <Segment testIDOnek="history-tab" secenekler={sekmeler} secili={sekme} onSec={setSekme} style={{ marginHorizontal: 16, marginBottom: 6 }} />
      <SectionList
        testID="history-list"
        sections={sections}
        keyExtractor={(r) => r.key}
        stickySectionHeadersEnabled
        contentContainerStyle={{ paddingBottom: 24, maxWidth: 720, width: '100%', alignSelf: 'center' }}
        refreshControl={
          <RefreshControl
            refreshing={syncing || (sekme === 'fis' && loading)}
            onRefresh={() => {
              syncNow();
              void refresh();
              void invoices.refetch();
              void tasksQ.refetch();
            }}
            tintColor={c.brandText}
          />
        }
        renderSectionHeader={({ section }) => (
          <View style={[styles.sec, { backgroundColor: c.bg }]}>
            <Text style={[metin.cap, { color: c.text3 }]}>{section.baslik}</Text>
          </View>
        )}
        ListEmptyComponent={
          busy ? (
            <View style={{ alignItems: 'center', paddingVertical: 40 }}>
              <KristalYukleyici etiket="Geçmiş yükleniyor" />
            </View>
          ) : (
            <BosDurum
              baslik={sekme === 'fis' ? 'Henüz fiş yok' : sekme === 'fatura' ? 'Fatura yok' : 'İstek yok'}
              aciklama={sekme === 'fis' ? 'Masraf, yakıt fişi ya da Z raporu çekin; müşavirinize gönderelim.' : undefined}
              eylem={sekme === 'fis' ? <Button title="Fiş çek" onPress={() => router.push('/capture')} /> : undefined}
            />
          )
        }
        renderItem={({ item: r, index }) => (
          <Satir
            testID="history-row"
            ilk={index === 0}
            sol={<TacRozeti durum={r.tac} />}
            baslik={r.baslik}
            alt={r.alt}
            altRenk={tone(r.ton)}
            sag={r.tutar !== null ? <Text style={[metin.sm, metin.num, { fontFamily: 'Inter_700Bold', color: c.text }]}>{tl(r.tutar)}</Text> : undefined}
            onPress={
              r.yerelId
                ? r.yerelId.startsWith('task:')
                  ? () => router.push({ pathname: '/task/[uuid]', params: { uuid: r.yerelId!.slice(5) } })
                  : () => router.push({ pathname: '/receipts/[id]', params: { id: r.yerelId! } })
                : belgeler.has(r.key)
                  ? () => belgeyiAc(belgeler.get(r.key)!)
                  : undefined
            }
          />
        )}
      />
    </SafeAreaView>
  );
}

const styles = StyleSheet.create({
  sec: { paddingHorizontal: 20, paddingTop: 12, paddingBottom: 6 },
});

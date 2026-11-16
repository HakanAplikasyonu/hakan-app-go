import { KristalYukleyici, TacRozeti, type TacDurumu } from '@hakan/mobile-core/hakan';
import { Ikon } from '@hakan/mobile-core/ui/ikon';
import { yerlesim } from '@hakan/mobile-core/ui/layout';
import { GorunumAyari, Kart, KesikKose, metin, Satir } from '@hakan/mobile-core/ui/parcalar';
import { useTheme } from '@hakan/mobile-core/ui/theme';
import { useRouter } from 'expo-router';
import { useEffect, useState } from 'react';
import { Pressable, RefreshControl, ScrollView, StyleSheet, Text, useWindowDimensions, View } from 'react-native';
import { SafeAreaView } from 'react-native-safe-area-context';

import { Button, Notice } from '@/components/ui';
import { errorMessage } from '@/core/api/envelope';
import { useAuth } from '@/core/auth/auth-provider';
import { enablePush, getPushPermission, type PushPermission } from '@/core/push/push';
import { initials } from '@/features/clients/format';
import { useDashboard, usePeriodCrown, type DashboardUpload } from '@/features/dashboard/api';
import {
  clock,
  countLabel,
  daysUntilVatDue,
  declarationPeriod,
  dueText,
  greeting,
  periodCrown,
  uploadKindLabel,
  type UploadKind,
} from '@/features/dashboard/format';
import { queueEta } from '@/features/review/format';

/**
 * Pano (MusavirMobil · 1): dönem tacı, birincil kart olarak onay kuyruğu
 * (tahmini süreyle), üç sayaç ve bugün gelen yüklemeler.
 */
export default function HomeScreen() {
  const { me, office, offices, switchOffice, signOut, can } = useAuth();
  const { colors: c } = useTheme();
  const router = useRouter();
  const { width } = useWindowDimensions();
  const genis = yerlesim(width) !== 'telefon';
  const q = useDashboard();
  const d = q.data;
  const period = declarationPeriod();
  const crown = usePeriodCrown(period, can('tenant.periods.read'));
  const [push, setPush] = useState<PushPermission | null>(null);
  useEffect(() => {
    getPushPermission()
      .then(setPush)
      .catch(() => setPush('unavailable'));
  }, []);

  const tac = crown.data ? periodCrown(crown.data.items, period) : null;
  const total = crown.data?.total ?? tac?.total ?? 0;
  const tacDurum: TacDurumu = !tac || !total ? 'bekleniyor' : tac.ready >= total ? 'onaylandi' : tac.ready > 0 ? 'yuklendi' : 'bekleniyor';
  const pending = d?.review?.pending ?? 0;

  return (
    <SafeAreaView style={{ flex: 1, backgroundColor: c.bg }} edges={['top', 'left', 'right']}>
      <ScrollView
        contentContainerStyle={[styles.body, genis ? styles.bodyWide : null]}
        refreshControl={<RefreshControl refreshing={q.isRefetching} onRefresh={() => void q.refetch()} tintColor={c.brandText} />}>
        <View style={styles.hd}>
          <View style={{ flex: 1 }}>
            <Text style={[metin.sm, { color: c.text3 }]}>{greeting()}</Text>
            <Text accessibilityRole="header" style={[styles.name, { color: c.text }]} numberOfLines={1}>
              {me?.user.name || office?.organization.name}
            </Text>
          </View>
          <View style={[styles.av, { backgroundColor: c.text }]} accessibilityLabel={office?.organization.name}>
            <Text style={[styles.avText, { color: c.bg }]}>{initials(office?.organization.name ?? '')}</Text>
          </View>
        </View>

        {push === 'undetermined' ? (
          <Kart style={{ padding: 16, gap: 8 }}>
            <Text style={[metin.sm, { fontFamily: 'Inter_600SemiBold', color: c.text }]}>Bildirimleri açın</Text>
            <Text style={[metin.xs, { color: c.text2 }]}>Mükellef belge yüklediğinde ve kritik uyarı açıldığında telefonunuza haber verelim.</Text>
            <Button
              title="Bildirimleri aç"
              onPress={() =>
                void enablePush().then((r) => setPush(r === 'registered' ? 'granted' : r === 'denied' ? 'denied' : 'unavailable'))
              }
            />
          </Kart>
        ) : null}

        {q.isLoading ? (
          <View style={{ alignItems: 'center', paddingVertical: 32 }}>
            <KristalYukleyici etiket="Pano yükleniyor" />
          </View>
        ) : null}
        {q.error ? <Notice>{errorMessage(q.error)}</Notice> : null}

        {d ? (
          <View style={genis ? styles.cols : styles.col}>
            <View style={[styles.col, genis ? { flex: 1 } : null]}>
              {/* Dönem tacı: kesik köşeli vurgulu kart */}
              <View
                testID="period-crown"
                accessible
                accessibilityLabel={tac ? `${period} dönemi: ${total} mükellefin ${tac.ready} tanesi beyana hazır, ${dueText(daysUntilVatDue())}` : `${period} dönemi`}
                style={[styles.crown, { backgroundColor: c.surface, borderColor: c.line }]}>
                <View style={{ flex: 1, gap: 4 }}>
                  <Text style={[metin.cap, { color: c.text3 }]}>
                    {period}
                    {total ? ` · ${total} mükellef` : ''}
                  </Text>
                  <Text style={[metin.kpi, { fontSize: 34, lineHeight: 40, color: c.text }]}>
                    {tac ? tac.ready : '—'}
                    <Text style={{ fontSize: 18, color: c.text3 }}>/{total || '—'}</Text>
                  </Text>
                  <Text style={[metin.xs, { color: c.text2 }]}>beyana hazır · {dueText(daysUntilVatDue())}</Text>
                </View>
                <TacRozeti durum={tacDurum} boyut={30} />
                <KesikKose zemin={c.bg} />
              </View>

              {/* Birincil eylem: onay kuyruğu */}
              {d.review ? (
                <Pressable
                  testID="stat-review"
                  accessibilityRole="button"
                  accessibilityLabel={`Onay kuyruğu: ${countLabel(pending)} öneri${pending ? `, tahmini ${queueEta(pending)}` : ''}`}
                  onPress={() => router.push('/review')}
                  style={({ pressed }) => [styles.primary, { backgroundColor: c.brand, opacity: pressed ? 0.9 : 1 }]}>
                  <View style={{ flex: 1, gap: 2 }}>
                    <Text style={[styles.primaryTitle, { color: c.onBrand }]}>Onay kuyruğu · {pending > 999 ? '999+' : pending}</Text>
                    <Text style={[metin.xs, { color: c.onBrand, opacity: 0.85 }]}>
                      {pending ? `Ortalama 6 sn / öneri · ${queueEta(pending)}` : 'Bekleyen öneri yok'}
                    </Text>
                  </View>
                  <Ikon ad="ileri" boyut={22} renk={c.onBrand} />
                </Pressable>
              ) : null}

              <View style={styles.stats}>
                <Stat
                  testID="stat-tasks"
                  label="Belge görevi"
                  value={d.doc_tasks ? d.doc_tasks.open + d.doc_tasks.submitted : null}
                  hint={d.doc_tasks?.overdue ? `${d.doc_tasks.overdue} gecikmiş` : undefined}
                  onPress={d.doc_tasks ? () => router.push('/gorevler') : undefined}
                />
                <Stat
                  testID="stat-warnings"
                  label="Engel"
                  value={d.warnings ? d.warnings.blocker : null}
                  danger={!!d.warnings?.blocker}
                  hint={d.warnings && d.warnings.open > d.warnings.blocker ? `${d.warnings.open - d.warnings.blocker} uyarı` : undefined}
                  onPress={d.warnings ? () => router.push('/warnings') : undefined}
                />
                <Stat testID="stat-uploads" label="Bugün" value={d.uploads.total} />
              </View>
            </View>

            <Kart style={genis ? { flex: 1 } : undefined}>
              <Text accessibilityRole="header" style={[metin.sm, styles.cardHead, { color: c.text }]}>
                Bugün gelen yüklemeler
              </Text>
              {d.uploads.recent.length === 0 ? (
                <Text style={[metin.xs, { color: c.text3, paddingHorizontal: 14, paddingBottom: 14 }]}>Bugün henüz yükleme yok.</Text>
              ) : (
                d.uploads.recent.map((u) => <UploadRow key={`${u.kind}-${u.uuid}`} upload={u} />)
              )}
              {d.uploads.total > d.uploads.recent.length ? (
                <Text style={[metin.xs, { color: c.text3, padding: 14 }]}>
                  Son {d.uploads.recent.length} yükleme gösteriliyor ({d.uploads.total} toplam).
                </Text>
              ) : null}
            </Kart>
          </View>
        ) : null}

        <View style={{ height: 8 }} />
        <Kart style={{ padding: 16 }}>
          <GorunumAyari />
        </Kart>
        {offices.length > 1 ? <Button title="Ofis değiştir" variant="secondary" onPress={switchOffice} /> : null}
        <Button title="Çıkış yap" variant="ghost" onPress={signOut} />
      </ScrollView>
    </SafeAreaView>
  );
}

function Stat({
  label,
  value,
  hint,
  danger,
  onPress,
  testID,
}: {
  label: string;
  value: number | null;
  hint?: string;
  danger?: boolean;
  onPress?: () => void;
  testID?: string;
}) {
  const { colors: c } = useTheme();
  const v = value === null ? '—' : value > 999 ? '999+' : String(value);
  return (
    <Pressable
      testID={testID}
      accessibilityRole={onPress ? 'button' : 'summary'}
      accessibilityLabel={`${label}: ${value === null ? 'yetkiniz yok' : v}${hint ? `, ${hint}` : ''}`}
      onPress={onPress}
      disabled={!onPress}
      style={({ pressed }) => [styles.stat, { backgroundColor: pressed ? c.surface2 : c.surface, borderColor: c.line }]}>
      <Text style={[metin.xs, { color: c.text2 }]} numberOfLines={1}>
        {label}
      </Text>
      <Text style={[metin.kpi, { fontSize: 22, lineHeight: 28, color: danger ? c.danger : c.text }]}>{v}</Text>
      {hint ? (
        <Text style={[metin.xs, { color: c.text3 }]} numberOfLines={1}>
          {hint}
        </Text>
      ) : null}
    </Pressable>
  );
}

function UploadRow({ upload }: { upload: DashboardUpload }) {
  const router = useRouter();
  const kind = uploadKindLabel[upload.kind as UploadKind] ?? upload.kind;
  return (
    <Satir
      testID="upload-row"
      sol={<TacRozeti durum="yuklendi" />}
      baslik={upload.taxpayer_title}
      alt={`${kind} · ${clock(upload.created_at)}`}
      accessibilityLabel={`${upload.taxpayer_title}, ${kind}, ${clock(upload.created_at)}`}
      onPress={() =>
        router.push({ pathname: '/clients/[engagement]', params: { engagement: upload.engagement_uuid, title: upload.taxpayer_title } })
      }
    />
  );
}

const styles = StyleSheet.create({
  body: { padding: 16, paddingTop: 4, gap: 12, flexGrow: 1 },
  bodyWide: { paddingHorizontal: 28, maxWidth: 1100, width: '100%', alignSelf: 'center' },
  hd: { flexDirection: 'row', alignItems: 'center', gap: 10, paddingHorizontal: 4, paddingTop: 12, paddingBottom: 6 },
  name: { fontFamily: 'InterTight_700Bold', fontSize: 24, lineHeight: 30 },
  av: { width: 40, height: 40, borderRadius: 10, alignItems: 'center', justifyContent: 'center' },
  avText: { fontFamily: 'Inter_700Bold', fontSize: 14 },
  col: { gap: 12 },
  cols: { flexDirection: 'row', gap: 16, alignItems: 'flex-start' },
  crown: { flexDirection: 'row', alignItems: 'flex-end', gap: 14, padding: 18, borderRadius: 14, borderWidth: StyleSheet.hairlineWidth, overflow: 'hidden' },
  primary: { flexDirection: 'row', alignItems: 'center', gap: 14, padding: 16, borderRadius: 14, minHeight: 64 },
  primaryTitle: { fontFamily: 'Inter_700Bold', fontSize: 17, lineHeight: 22 },
  stats: { flexDirection: 'row', gap: 10 },
  stat: { flex: 1, minHeight: 72, borderWidth: StyleSheet.hairlineWidth, borderRadius: 14, padding: 12, gap: 2 },
  cardHead: { fontFamily: 'Inter_600SemiBold', paddingHorizontal: 14, paddingVertical: 12 },
});

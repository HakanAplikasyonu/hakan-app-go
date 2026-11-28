import { unreadCount } from '@hakan/mobile-core/notifications/inbox';
import { KristalYukleyici, TacRozeti } from '@hakan/mobile-core/hakan';
import { Ikon } from '@hakan/mobile-core/ui/ikon';
import { DOKUNMA, yerlesim } from '@hakan/mobile-core/ui/layout';
import { Hap, Kart, KesikKose, metin, Satir } from '@hakan/mobile-core/ui/parcalar';
import { useTheme } from '@hakan/mobile-core/ui/theme';
import { router } from 'expo-router';
import { useCallback, useEffect, useState } from 'react';
import { Pressable, RefreshControl, ScrollView, StyleSheet, Text, useWindowDimensions, View } from 'react-native';
import { SafeAreaView } from 'react-native-safe-area-context';

import { Button } from '@/components/ui';
import { useAuth } from '@/core/auth/auth-provider';
import { initials, roleLabel } from '@/core/auth/taxpayers';
import { enablePush, getPushPermission, type PushPermission } from '@/core/push/push';
import { dueLine, lateBadge, waitingLine } from '@/features/home/crown';
import { KdvDurumuKarti, YabanciFaturaKarti } from '@/features/kdv/kartlar';
import { useInbox } from '@/features/notifications/api';
import { useReceipts } from '@/features/receipts/provider';
import { bekleyenIstekMetni, bekleyenIstekSayisi } from '@/features/tasks/tasks';
import { openTask, useTasks } from '@/features/tasks/use-tasks';

/**
 * Ana sayfa (MukellefMobil · 1): büyük "Fiş / Z raporu çek" kartı,
 * "Müşavirinin istedikleri" ve çevrimdışı kuyruk durumu.
 */
export default function HomeScreen() {
  const { me, taxpayer, taxpayers, switchTaxpayer } = useAuth();
  const { colors: c } = useTheme();
  const { width } = useWindowDimensions();
  const genis = yerlesim(width) !== 'telefon';
  const { summary, online, retryFailed } = useReceipts();
  const { tasks, query } = useTasks();
  const inbox = useInbox();
  const unread = unreadCount(inbox.items);
  const [push, setPush] = useState<PushPermission>('unavailable');
  const [pushBusy, setPushBusy] = useState(false);

  useEffect(() => {
    getPushPermission().then(setPush);
  }, []);

  const turnOnPush = useCallback(async () => {
    setPushBusy(true);
    try {
      await enablePush();
      setPush(await getPushPermission());
    } finally {
      setPushBusy(false);
    }
  }, []);

  if (!taxpayer) return null;
  const t = taxpayer.taxpayer;
  const requests = tasks.filter((x) => x.source === 'server');
  const late = lateBadge(requests);
  const bekleyen = bekleyenIstekMetni(bekleyenIstekSayisi(query.data));
  const waiting = summary.waiting + summary.blocked;

  return (
    <SafeAreaView style={{ flex: 1, backgroundColor: c.bg }} edges={['top', 'left', 'right']}>
      <ScrollView
        contentContainerStyle={[styles.body, genis ? styles.bodyWide : null]}
        refreshControl={<RefreshControl refreshing={query.isRefetching} onRefresh={() => void query.refetch()} tintColor={c.brandText} />}>
        <View style={styles.hd}>
          <Pressable
            accessibilityRole="button"
            accessibilityLabel={taxpayers.length > 1 ? `Şirket: ${t.title}. Değiştir` : `Şirket: ${t.title}`}
            onPress={switchTaxpayer}
            disabled={taxpayers.length < 2}
            style={{ flexDirection: 'row', alignItems: 'center', gap: 10, flex: 1, minHeight: DOKUNMA }}>
            <View style={[styles.av, { backgroundColor: c.brand }]}>
              <Text style={[styles.avText, { color: c.onBrand }]}>{initials(t.title)}</Text>
            </View>
            <View style={{ flex: 1 }}>
              <Text style={[metin.govde, { fontFamily: 'Inter_700Bold', color: c.text }]} numberOfLines={1}>
                {t.title}
              </Text>
              <Text style={[metin.xs, metin.num, { color: c.text3 }]} numberOfLines={1}>
                {t.tax_id_type.toUpperCase()} {t.tax_id} · {roleLabel(taxpayer.role)}
                {taxpayers.length > 1 ? ' · değiştir' : ''}
              </Text>
            </View>
          </Pressable>
          <Pressable
            testID="open-notifications"
            accessibilityRole="button"
            accessibilityLabel={unread ? `Bildirimler, ${unread} okunmamış` : 'Bildirimler'}
            onPress={() => router.push('/notifications')}
            style={styles.icon}>
            <Ikon ad="bildirim" renk={c.text} />
            {unread ? <View style={[styles.dot, { backgroundColor: c.danger }]} /> : null}
          </Pressable>
        </View>

        <Text accessibilityRole="header" style={[styles.hello, { color: c.text }]}>
          Merhaba{me?.user.name ? `, ${me.user.name}` : ''}
        </Text>

        {/* Büyük çekim düğmesi: kesik köşeli marka kartı */}
        <Pressable
          testID="capture-card"
          accessibilityRole="button"
          accessibilityLabel="Fiş / Z raporu çek"
          accessibilityHint="Kamerayı açar"
          onPress={() => router.push('/capture')}
          style={({ pressed }) => [styles.capture, { backgroundColor: c.brand, opacity: pressed ? 0.9 : 1 }]}>
          <View style={styles.capIcon}>
            <Ikon ad="fisCek" boyut={22} renk={c.onBrand} />
          </View>
          <View style={{ gap: 4 }}>
            <Text style={[styles.capTitle, { color: c.onBrand }]}>Fiş / Z raporu çek</Text>
            <Text style={[metin.sm, { color: c.onBrand, opacity: 0.85 }]}>Çek, gönder, gerisini müşavirin halleder.</Text>
          </View>
          <KesikKose zemin={c.bg} />
        </Pressable>

        {/* #780: ofisin açık belge istekleri; hiç yoksa kart görünmez. */}
        {bekleyen ? (
          <Pressable
            testID="home-doc-requests"
            accessibilityRole="button"
            accessibilityLabel={`${bekleyen}. İstekleri aç`}
            onPress={() => router.push('/tasks')}
            style={({ pressed }) => [styles.queue, styles.docCard, { backgroundColor: pressed ? c.surface2 : c.surface, borderColor: late ? c.danger : c.line }]}>
            <Ikon ad="uyari" renk={late ? c.danger : c.warn} />
            <View style={{ flex: 1, gap: 2 }}>
              <Text style={[metin.sm, { fontFamily: 'Inter_700Bold', color: c.text }]}>{bekleyen}</Text>
              <Text style={[metin.xs, { color: late ? c.danger : c.text3 }]}>
                {late ? `${late}. ` : ''}Müşaviriniz bu belgeleri bekliyor; dokunun, fotoğraf ya da PDF yükleyin.
              </Text>
            </View>
            <Ikon ad="sag" boyut={16} renk={c.text3} />
          </Pressable>
        ) : null}

        <Kart testID="home-requests">
          <View style={styles.cardHead}>
            <Text accessibilityRole="header" style={[metin.govde, { fontFamily: 'Inter_700Bold', color: c.text, flex: 1 }]}>
              Müşavirinin istedikleri
            </Text>
            {late ? <Hap metin={late} ton="danger" /> : null}
          </View>
          {requests.length === 0 ? (
            <Text style={[metin.sm, { color: c.text3, paddingHorizontal: 14, paddingBottom: 14 }]}>Şu an istenen bir belge yok.</Text>
          ) : (
            requests.slice(0, 3).map((r) => {
              const due = dueLine(r.due);
              return (
                <Satir
                  key={r.id}
                  testID="home-request-row"
                  sol={<TacRozeti durum={r.overdue ? 'sorun' : 'bekleniyor'} />}
                  baslik={r.title}
                  alt={[due?.text, r.detail.split('\n')[0]].filter(Boolean).join(' · ') || undefined}
                  altRenk={due?.late ? c.danger : undefined}
                  sag={<Ikon ad="sag" boyut={16} renk={c.text3} />}
                  onPress={() => openTask(r)}
                />
              );
            })
          )}
          {requests.length > 3 ? (
            <Pressable accessibilityRole="button" onPress={() => router.push('/tasks')} style={[styles.more, { borderTopColor: c.line }]}>
              <Text style={[metin.sm, { fontFamily: 'Inter_600SemiBold', color: c.brandText }]}>Tüm istekler ({requests.length})</Text>
            </Pressable>
          ) : null}
        </Kart>

        {/* #780: ofis kapattıysa ikisi de görünmez. */}
        <KdvDurumuKarti />
        <YabanciFaturaKarti />

        {waiting > 0 || !online ? (
          <Kart testID="queue-card" style={styles.queue}>
            {waiting > 0 ? <KristalYukleyici boyut={22} gecikme={0} etiket="Fişler telefonda bekliyor" /> : <Ikon ad="uyari" renk={c.warn} />}
            <View style={{ flex: 1, gap: 2 }}>
              <Text style={[metin.sm, { fontFamily: 'Inter_600SemiBold', color: c.text }]}>
                {waitingLine(waiting) ?? 'Bağlantı yok'}
              </Text>
              <Text style={[metin.xs, { color: c.text3 }]}>
                {online
                  ? summary.blocked
                    ? 'Şirketinizin bağlı bir müşavirlik ofisi yok ya da sunucu şu an fiş kabul etmiyor; kendiliğinden gönderilecek.'
                    : 'Gönderiliyor…'
                  : 'Bağlantı gelince kendiliğinden gönderilecek'}
              </Text>
            </View>
          </Kart>
        ) : null}

        {summary.failed > 0 ? (
          <Kart testID="queue-failed-card" style={{ padding: 16, gap: 10 }}>
            <Text style={[metin.sm, { fontFamily: 'Inter_600SemiBold', color: c.text }]}>{summary.failed} fiş gönderilemedi</Text>
            <Text style={[metin.xs, { color: c.text2 }]}>
              Fotoğraflar telefonda duruyor, müşavirinize henüz ulaşmadı. Bağlantınız iyiyken bir daha gönderin.
            </Text>
            <Button title="Tekrar gönder" variant="secondary" onPress={retryFailed} testID="queue-retry-failed" />
          </Kart>
        ) : null}

        {push === 'undetermined' ? (
          <Kart style={{ padding: 16, gap: 10 }}>
            <Text style={[metin.sm, { fontFamily: 'Inter_600SemiBold', color: c.text }]}>Bildirimleri açın</Text>
            <Text style={[metin.xs, { color: c.text2 }]}>Müşaviriniz belge istediğinde veya bir kayıt onayınızı beklediğinde haber verelim.</Text>
            <Button title="Bildirimleri aç" onPress={turnOnPush} loading={pushBusy} />
          </Kart>
        ) : null}
      </ScrollView>
    </SafeAreaView>
  );
}

const styles = StyleSheet.create({
  body: { padding: 16, paddingTop: 4, gap: 12, flexGrow: 1 },
  bodyWide: { paddingHorizontal: 28, maxWidth: 720, width: '100%', alignSelf: 'center' },
  hd: { flexDirection: 'row', alignItems: 'center', gap: 10, paddingTop: 12, paddingBottom: 4 },
  av: { width: 40, height: 40, borderRadius: 10, alignItems: 'center', justifyContent: 'center' },
  avText: { fontFamily: 'Inter_700Bold', fontSize: 14 },
  icon: { width: DOKUNMA, height: DOKUNMA, alignItems: 'center', justifyContent: 'center', borderRadius: 10 },
  dot: { position: 'absolute', top: 10, right: 11, width: 8, height: 8, borderRadius: 4 },
  hello: { fontFamily: 'InterTight_700Bold', fontSize: 22, lineHeight: 28, paddingTop: 4 },
  capture: { borderRadius: 20, padding: 22, gap: 18, minHeight: 170, overflow: 'hidden' },
  capIcon: { width: 48, height: 48, borderRadius: 14, backgroundColor: 'rgba(255,255,255,0.18)', alignItems: 'center', justifyContent: 'center' },
  capTitle: { fontFamily: 'InterTight_700Bold', fontSize: 24, lineHeight: 30 },
  cardHead: { flexDirection: 'row', alignItems: 'center', gap: 8, padding: 14 },
  more: { minHeight: DOKUNMA, alignItems: 'center', justifyContent: 'center', borderTopWidth: StyleSheet.hairlineWidth },
  queue: { flexDirection: 'row', alignItems: 'center', gap: 12, padding: 14 },
  docCard: { borderRadius: 14, borderWidth: 1, minHeight: DOKUNMA },
});

import { KristalYukleyici, TacRozeti } from '@hakan/mobile-core/hakan';
import { Ikon } from '@hakan/mobile-core/ui/ikon';
import { DOKUNMA } from '@hakan/mobile-core/ui/layout';
import { Hap, Kart, metin } from '@hakan/mobile-core/ui/parcalar';
import { useTheme } from '@hakan/mobile-core/ui/theme';
import * as DocumentPicker from 'expo-document-picker';
import { Image } from 'expo-image';
import * as ImagePicker from 'expo-image-picker';
import { useState } from 'react';
import { Linking, Pressable, RefreshControl, ScrollView, StyleSheet, Text, View } from 'react-native';

import { Notice } from '@/components/ui';
import { errorMessage } from '@/core/api/envelope';
import { useAuth } from '@/core/auth/auth-provider';
import { belgeyiAc } from '@/features/documents/ac';
import { dueLine, taskCrown } from '@/features/home/crown';
import { statusLabel } from '@/features/receipts/fields';
import { useReceipts } from '@/features/receipts/provider';
import { taskItems } from '@/features/receipts/queue';

import { AZAMI_DOSYA_BAYT, donemEtiketi, isBankTask, moneyText, taskStatusLabel, taskTitle, type ServerTask } from './tasks';
import { useTask } from './use-tasks';

const dayLabel = (iso: string) => new Intl.DateTimeFormat('tr-TR', { day: 'numeric', month: 'long', timeZone: 'Europe/Istanbul' }).format(new Date(iso));

/**
 * İstek ayrıntısı (MukellefMobil · 3; tablette MukellefTablet sağ paneli):
 * ne istendiği, kim istedi, son gün, not, eklenenler ve fotoğraf/dosya ekleme.
 * Seçilen fotoğraflar telefondaki kuyruğa girer ve bağlantı olunca gönderilir.
 */
export function IstekAyrintisi({ uuid, genis = false }: { uuid: string; genis?: boolean }) {
  const { colors: c } = useTheme();
  const task = useTask(uuid);
  const { items, enqueueTask, online, retry } = useReceipts();
  const { taxpayer } = useAuth();
  const [denied, setDenied] = useState(false);
  const [saving, setSaving] = useState(false);
  const [tooBig, setTooBig] = useState(false);
  const queued = taskItems(items, taxpayer?.taxpayer.uuid ?? '', uuid).filter((i) => i.status !== 'uploaded');
  const t = task.data;
  const closed = t ? t.status === 'matched' || t.status === 'cancelled' : false;
  const bank = t ? isBankTask(t) : false;
  const due = t && !closed ? dueLine(t.due_at) : null;

  async function add(fromCamera: boolean) {
    if (fromCamera) {
      const perm = await ImagePicker.requestCameraPermissionsAsync();
      if (!perm.granted) {
        setDenied(true);
        return;
      }
    }
    const res = fromCamera
      ? await ImagePicker.launchCameraAsync({ mediaTypes: ['images'], allowsEditing: true, quality: 0.7 })
      : await ImagePicker.launchImageLibraryAsync({ mediaTypes: ['images'], allowsMultipleSelection: true, selectionLimit: 10, quality: 0.7 });
    if (res.canceled) return;
    setSaving(true);
    try {
      await enqueueTask(
        uuid,
        res.assets.map((a) => ({ uri: a.uri, mimeType: a.mimeType })),
      );
    } finally {
      setSaving(false);
    }
  }

  /**
   * PDF seçimi (#780): dosya kuyruğa girer, fotoğraflar gibi gönderilir. Belge
   * isteğine banka ekstresi dosyası da eklenir (Excel, CSV, MT940 metni); banka
   * hareketinin belgesi yalnızca PDF ya da fotoğraftır.
   */
  async function addPdf() {
    setTooBig(false);
    const res = await DocumentPicker.getDocumentAsync({ type: bank ? 'application/pdf' : DOSYA_TURLERI, copyToCacheDirectory: true });
    const a = res.canceled ? undefined : res.assets[0];
    if (!a) return;
    if (a.size && a.size > AZAMI_DOSYA_BAYT) {
      setTooBig(true);
      return;
    }
    setSaving(true);
    try {
      const pdf = a.name.toLowerCase().endsWith('.pdf');
      await enqueueTask(uuid, [{ uri: a.uri, mimeType: a.mimeType ?? (pdf || bank ? 'application/pdf' : 'application/octet-stream') }]);
    } finally {
      setSaving(false);
    }
  }

  const heading = t ? (bank ? `${dayLabel(t.bank_transaction.transaction_date)} tarihli ${moneyText(t.bank_transaction.amount, t.bank_transaction.currency)} hareketin belgesi` : taskTitle(t)) : '';
  const who = t && bank ? t.bank_transaction.counterparty_name || t.bank_transaction.description : null;

  return (
    <ScrollView
      contentContainerStyle={[styles.body, genis ? { padding: 36, gap: 18 } : null]}
      refreshControl={<RefreshControl refreshing={task.isRefetching} onRefresh={() => void task.refetch()} tintColor={c.brandText} />}>
      {task.isLoading ? (
        <View style={{ alignItems: 'center', paddingVertical: 40 }}>
          <KristalYukleyici etiket="İstek yükleniyor" />
        </View>
      ) : null}
      {task.isError ? <Notice>{errorMessage(task.error)}</Notice> : null}
      {t ? (
        <>
          <View style={{ gap: 8 }}>
            <View style={{ flexDirection: 'row', gap: 8, alignItems: 'center' }}>
              <Hap metin={bank ? taskTitle(t) : 'Belge isteği'} />
              <TacRozeti durum={taskCrown(t.status, t.overdue)} etiket metin={t.overdue && !closed ? 'Gecikti' : taskStatusLabel(t.status)} />
            </View>
            <Text accessibilityRole="header" style={[styles.h2, genis ? { fontSize: 28, lineHeight: 34 } : null, { color: c.text }]}>
              {heading}
            </Text>
            {who ? (
              <Text style={[metin.govde, { color: c.text2 }]}>
                Banka hesabınızdan <Text style={{ fontFamily: 'Inter_700Bold', color: c.text }}>{who}</Text> ile ilgili hareketin faturası ya da makbuzu.
              </Text>
            ) : null}
          </View>

          <Kart style={{ padding: 14, gap: 10 }}>
            {t.organization_name ? <Bilgi k="İsteyen" v={t.organization_name} /> : null}
            {t.period ? <Bilgi k="Dönem" v={donemEtiketi(t.period)} /> : null}
            <Bilgi k="Durum" v={t.overdue && !closed ? 'Gecikti' : taskStatusLabel(t.status)} renk={t.overdue && !closed ? c.danger : undefined} />
            <Bilgi k="Son gün" v={`${dayLabel(t.due_at)}${due ? ` · ${due.text}` : ''}`} renk={due?.late ? c.danger : undefined} />
            {t.note ? <Bilgi k="Not" v={t.note} /> : null}
          </Kart>

          {closed && (t.uploads ?? []).length ? (
            <View style={{ gap: 8 }}>
              <Text style={[metin.cap, { color: c.text3 }]}>Gönderilen</Text>
              <View style={styles.thumbs}>
                {(t.uploads ?? []).map((u) => (
                  <GonderilenDosya key={u.uuid} taskUuid={uuid} u={u} />
                ))}
              </View>
            </View>
          ) : null}
          {closed ? (
            <Notice tone="info">
              {t.status === 'matched'
                ? bank
                  ? 'Belge alındı ve hareketle eşleşti. Teşekkürler.'
                  : 'Müşaviriniz belgeyi kabul etti. Teşekkürler.'
                : 'Müşaviriniz bu isteği kapattı.'}
            </Notice>
          ) : (
            <>
              <View style={{ gap: 8 }}>
                <Text style={[metin.cap, { color: c.text3 }]}>Eklenen</Text>
                <View style={styles.thumbs}>
                  {(t.uploads ?? []).map((u) => (
                    <GonderilenDosya key={u.uuid} taskUuid={uuid} u={u} />
                  ))}
                  {queued.map((i) => (
                    <Pressable
                      key={i.id}
                      accessibilityRole="button"
                      accessibilityLabel={`Telefonda: ${statusLabel(i)}. ${i.status === 'failed' ? 'Yeniden göndermek' : 'Açmak'} için dokunun.`}
                      onPress={
                        i.status === 'failed'
                          ? () => retry(i.id)
                          : () => belgeyiAc({ title: dosyaTuru(i.mimeType), localUri: i.fileUri, contentType: i.mimeType, id: i.id, prefix: 'kuyruk' })
                      }
                      style={[styles.thumb, { borderColor: i.status === 'failed' ? c.danger : c.line }]}>
                      {dosyaTuru(i.mimeType) !== 'Fotoğraf' ? (
                        <View style={[StyleSheet.absoluteFill, styles.sent]}>
                          <Ikon ad="klasor" renk={c.text3} />
                          <Text style={[metin.xs, { color: c.text3 }]}>{dosyaTuru(i.mimeType)}</Text>
                        </View>
                      ) : (
                        <Image source={{ uri: i.fileUri }} style={StyleSheet.absoluteFill} contentFit="cover" />
                      )}
                    </Pressable>
                  ))}
                  <Pressable
                    testID="task-add-photo"
                    accessibilityRole="button"
                    accessibilityLabel="Belgenin fotoğrafını çek"
                    disabled={saving}
                    onPress={() => void add(true)}
                    style={[styles.thumb, styles.add, { borderColor: c.lineStrong }]}>
                    <Ikon ad="fisCek" boyut={22} renk={c.text3} />
                  </Pressable>
                </View>
              </View>
              {denied ? (
                <Notice>
                  Kamera izni verilmedi.{' '}
                  <Text style={{ color: c.brandText }} onPress={() => Linking.openSettings()}>
                    Ayarları aç
                  </Text>
                </Notice>
              ) : null}
              <View style={{ flex: 1 }} />
              <View style={genis ? { flexDirection: 'row', gap: 12 } : { gap: 10 }}>
                <Pressable
                  testID="task-camera"
                  accessibilityRole="button"
                  disabled={saving}
                  onPress={() => void add(true)}
                  style={({ pressed }) => [styles.btn, genis ? { flex: 1 } : null, { backgroundColor: c.brand, opacity: saving ? 0.6 : pressed ? 0.85 : 1 }]}>
                  <Ikon ad="fisCek" renk={c.onBrand} />
                  <Text style={[metin.govde, { fontFamily: 'Inter_600SemiBold', color: c.onBrand }]}>Fotoğraf çek</Text>
                </Pressable>
                <Pressable
                  testID="task-gallery"
                  accessibilityRole="button"
                  disabled={saving}
                  onPress={() => void add(false)}
                  style={({ pressed }) => [styles.btn, genis ? { flex: 1 } : null, { borderWidth: 1, borderColor: c.lineStrong, backgroundColor: pressed ? c.surface2 : c.surface }]}>
                  <Ikon ad="klasor" renk={c.text} />
                  <Text style={[metin.govde, { fontFamily: 'Inter_600SemiBold', color: c.text }]}>Galeriden seç</Text>
                </Pressable>
                <Pressable
                  testID="task-pdf"
                  accessibilityRole="button"
                  accessibilityLabel={bank ? 'PDF dosyası seç' : 'PDF ya da Excel dosyası seç'}
                  disabled={saving}
                  onPress={() => void addPdf()}
                  style={({ pressed }) => [styles.btn, genis ? { flex: 1 } : null, { borderWidth: 1, borderColor: c.lineStrong, backgroundColor: pressed ? c.surface2 : c.surface }]}>
                  <Ikon ad="klasor" renk={c.text} />
                  <Text style={[metin.govde, { fontFamily: 'Inter_600SemiBold', color: c.text }]}>{bank ? 'PDF seç' : 'PDF / Excel seç'}</Text>
                </Pressable>
              </View>
              {tooBig ? <Notice>{"Dosya 20 MB'tan büyük. Daha küçük bir PDF seçin ya da sayfaların fotoğrafını çekin."}</Notice> : null}
              <Text style={[metin.xs, { color: c.text3, textAlign: 'center' }]}>
                {online ? 'Seçtiğiniz fotoğraf ve PDF hemen gönderilir; isteğin durumu kendiliğinden güncellenir.' : 'Bağlantı yok: dosyalar telefonda saklanır, bağlantı gelince gönderilir.'}
              </Text>
            </>
          )}
        </>
      ) : null}
    </ScrollView>
  );
}

/** Belge isteğine seçilebilen dosyalar: PDF ve banka ekstresi (Excel, CSV, MT940 metni). */
const DOSYA_TURLERI = [
  'application/pdf',
  'application/vnd.openxmlformats-officedocument.spreadsheetml.sheet',
  'application/vnd.ms-excel',
  'text/csv',
  'text/comma-separated-values',
  'text/plain',
];

/** Kuyruktaki dosyanın türü: fotoğraf, PDF ya da başka bir dosya (ekstre). */
function dosyaTuru(mimeType?: string | null): string {
  if (!mimeType || mimeType.startsWith('image/')) return 'Fotoğraf';
  return mimeType === 'application/pdf' ? 'PDF' : 'Dosya';
}

type Yukleme = NonNullable<ServerTask['uploads']>[number];

/** İsteğe gönderilmiş dosya (#780): dokununca uygulama içi görüntüleyicide açılır. */
function GonderilenDosya({ taskUuid, u }: { taskUuid: string; u: Yukleme }) {
  const { colors: c } = useTheme();
  const tur = u.kind === 'pdf' ? 'PDF' : u.kind === 'image' ? 'Fotoğraf' : 'Dosya';
  return (
    <Pressable
      testID="task-upload"
      accessibilityRole="button"
      accessibilityLabel={`Gönderildi: ${u.file_name ?? tur}. Açmak için dokunun.`}
      onPress={() =>
        belgeyiAc({
          title: u.file_name || tur,
          path: `/v1/taxpayer/document-tasks/${encodeURIComponent(taskUuid)}/uploads/${encodeURIComponent(u.uuid)}/file`,
          fileName: u.file_name,
          contentType: u.content_type ?? (u.kind === 'pdf' ? 'application/pdf' : null),
          size: u.size_bytes,
          id: u.uuid,
          prefix: 'istek',
        })
      }
      style={({ pressed }) => [styles.thumb, styles.sent, { borderColor: c.line, backgroundColor: pressed ? c.surface : c.surface2 }]}>
      <Ikon ad="onayla" renk={c.ok} />
      <Text style={[metin.xs, { color: c.text3 }]} numberOfLines={1}>
        {tur}
      </Text>
    </Pressable>
  );
}

function Bilgi({ k, v, renk }: { k: string; v: string; renk?: string }) {
  const { colors: c } = useTheme();
  return (
    <View style={{ flexDirection: 'row', gap: 12 }}>
      <Text style={[metin.sm, { color: c.text3, width: 72 }]}>{k}</Text>
      <Text style={[metin.sm, { fontFamily: 'Inter_600SemiBold', color: renk ?? c.text, flex: 1, textAlign: 'right' }]}>{v}</Text>
    </View>
  );
}

const styles = StyleSheet.create({
  body: { padding: 16, gap: 16, flexGrow: 1, maxWidth: 720, width: '100%', alignSelf: 'center' },
  h2: { fontFamily: 'InterTight_700Bold', fontSize: 24, lineHeight: 30 },
  thumbs: { flexDirection: 'row', flexWrap: 'wrap', gap: 8 },
  thumb: { width: 72, height: 92, borderRadius: 10, borderWidth: 1, overflow: 'hidden' },
  sent: { alignItems: 'center', justifyContent: 'center', gap: 4 },
  add: { borderStyle: 'dashed', borderWidth: 1.5, alignItems: 'center', justifyContent: 'center' },
  btn: { minHeight: 52, borderRadius: 12, flexDirection: 'row', gap: 8, alignItems: 'center', justifyContent: 'center', paddingHorizontal: 16, minWidth: DOKUNMA },
});

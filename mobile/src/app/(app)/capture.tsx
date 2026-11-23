import { Ikon } from '@hakan/mobile-core/ui/ikon';
import { DOKUNMA } from '@hakan/mobile-core/ui/layout';
import { metin } from '@hakan/mobile-core/ui/parcalar';
import { useTheme } from '@hakan/mobile-core/ui/theme';
import { colors as temaRenkleri } from '@hakan/mobile-core/ui/tokens';
import { Image } from 'expo-image';
import * as ImagePicker from 'expo-image-picker';
import { router, useLocalSearchParams } from 'expo-router';
import { useState } from 'react';
import { Linking, Pressable, ScrollView, StyleSheet, Text, View } from 'react-native';
import { SafeAreaView } from 'react-native-safe-area-context';

import { kindLabels } from '@/features/receipts/fields';
import { useReceipts, type CapturedPhoto } from '@/features/receipts/provider';
import { MAX_PARTS, type ReceiptKind } from '@/features/receipts/queue';

const KINDS: ReceiptKind[] = ['expense_receipt', 'fuel_receipt', 'z_report'];
const KISA: Record<ReceiptKind, string> = { expense_receipt: 'Gider fişi', fuel_receipt: 'Akaryakıt', z_report: 'Z raporu' };
const isKind = (v: unknown): v is ReceiptKind => typeof v === 'string' && (KINDS as string[]).includes(v);

/** Kamera ekranı her temada koyu (tasarım: MukellefMobil · 2). */

/**
 * Fiş çek (MukellefMobil · 2): fiş türü seçici, kenar kılavuzları, büyük
 * deklanşör, galeri ve seri çekim sayacı. Çekim sistem kamerasıyla yapılır
 * (kırpma ile); arka arkaya çekilen fotoğraflar seri olarak birikir.
 */
export default function CaptureScreen() {
  const params = useLocalSearchParams<{ kind?: string }>();
  const { enqueue, online } = useReceipts();
  const { colors: c, name } = useTheme();
  // Vizör her temada koyu (#367): renkler seçili paletin koyu token'larından.
  const kamera = temaRenkleri[name.startsWith('n3') ? 'n3-dark' : 'm3-dark'];
  const KOYU = kamera.bg;
  const BEYAZ = kamera.text;
  const [kind, setKind] = useState<ReceiptKind>(isKind(params.kind) ? params.kind : 'expense_receipt');
  const [photos, setPhotos] = useState<CapturedPhoto[]>([]);
  const [denied, setDenied] = useState(false);
  const [saving, setSaving] = useState(false);
  // Uzun Z raporu birkaç fotoğrafta: parça 1/2/3 tek Z raporu olarak gider.
  const [asParts, setAsParts] = useState(false);
  const partsOn = kind === 'z_report' && asParts && photos.length > 1;
  const last = photos[photos.length - 1];

  async function shoot() {
    const perm = await ImagePicker.requestCameraPermissionsAsync();
    if (!perm.granted) {
      setDenied(true);
      return;
    }
    const res = await ImagePicker.launchCameraAsync({ mediaTypes: ['images'], allowsEditing: true, quality: 0.7 });
    if (!res.canceled) setPhotos((p) => [...p, ...res.assets.map((a) => ({ uri: a.uri, mimeType: a.mimeType }))]);
  }

  async function pick() {
    const res = await ImagePicker.launchImageLibraryAsync({
      mediaTypes: ['images'],
      allowsMultipleSelection: true,
      selectionLimit: 20,
      quality: 0.7,
    });
    if (!res.canceled) setPhotos((p) => [...p, ...res.assets.map((a) => ({ uri: a.uri, mimeType: a.mimeType }))]);
  }

  async function send() {
    setSaving(true);
    try {
      await enqueue(kind, photos, partsOn);
      router.replace('/receipts');
    } finally {
      setSaving(false);
    }
  }

  return (
    <SafeAreaView style={{ flex: 1, backgroundColor: KOYU }} edges={['top', 'left', 'right', 'bottom']}>
      <View style={styles.hd}>
        <Pressable testID="capture-close" accessibilityRole="button" accessibilityLabel="Kapat" onPress={() => router.back()} style={styles.close}>
          <Ikon ad="kapat" renk={BEYAZ} />
        </Pressable>
        <View style={{ flex: 1 }} />
        <View style={styles.kinds} accessibilityRole="radiogroup">
          {KINDS.map((k) => {
            const on = k === kind;
            return (
              <Pressable
                key={k}
                testID={`capture-kind-${k}`}
                accessibilityRole="radio"
                accessibilityLabel={kindLabels[k]}
                accessibilityState={{ selected: on }}
                onPress={() => setKind(k)}
                hitSlop={{ top: 6, bottom: 6 }}
                style={[styles.kind, on ? { backgroundColor: BEYAZ } : null]}>
                <Text style={[styles.kindText, { color: on ? KOYU : BEYAZ }]}>{KISA[k]}</Text>
              </Pressable>
            );
          })}
        </View>
      </View>

      <View style={[styles.view, { backgroundColor: kamera.surface3 }]}>
        {last ? (
          <Image source={{ uri: last.uri }} style={StyleSheet.absoluteFill} contentFit="contain" accessibilityLabel={`Son çekilen fotoğraf, toplam ${photos.length}`} />
        ) : (
          <View style={styles.guide}>
            <Ikon ad="fisCek" boyut={28} renk="rgba(255,255,255,0.6)" />
            <Text style={[metin.sm, { color: 'rgba(255,255,255,0.75)', textAlign: 'center' }]}>
              {kind === 'z_report'
                ? 'Gün sonu Z raporunun tamamı kadrajda olsun.'
                : 'Fişi düz bir zemine koyun; toplam, tarih ve VKN okunur olsun. Çektikten sonra kenarları kırpabilirsiniz.'}
            </Text>
          </View>
        )}
        {(['tl', 'tr', 'bl', 'br'] as const).map((k) => (
          <View key={k} pointerEvents="none" style={[styles.corner, cornerStyle[k], { borderColor: c.crystal.mid }]} />
        ))}
        {photos.length ? (
          <View style={[styles.badge, { backgroundColor: c.brand }]}>
            <Text style={[metin.xs, { color: c.onBrand, fontFamily: 'Inter_600SemiBold' }]}>
              {photos.length} fotoğraf · {online ? 'gönder deyince gider' : 'bağlantı gelince gider'}
            </Text>
          </View>
        ) : null}
      </View>

      {denied ? (
        <Text style={[metin.xs, { color: BEYAZ, textAlign: 'center', paddingHorizontal: 24 }]}>
          Kamera izni verilmedi. Galeriden seçebilir ya da{' '}
          <Text style={{ textDecorationLine: 'underline' }} onPress={() => Linking.openSettings()}>
            ayarlardan izin verebilirsiniz
          </Text>
          .
        </Text>
      ) : null}

      {kind === 'z_report' && photos.length > 1 ? (
        <Pressable
          testID="capture-parts"
          accessibilityRole="switch"
          accessibilityLabel="Fotoğrafları tek Z raporu olarak birleştir"
          accessibilityState={{ checked: asParts }}
          onPress={() => setAsParts((v) => !v)}
          style={[styles.parts, { borderColor: asParts ? c.brand : 'rgba(255,255,255,0.3)' }]}>
          <View style={[styles.check, { backgroundColor: asParts ? c.brand : 'transparent', borderColor: asParts ? c.brand : BEYAZ }]}>
            {asParts ? <Ikon ad="onayla" boyut={14} renk={c.onBrand} /> : null}
          </View>
          <Text style={[metin.xs, { color: BEYAZ, flex: 1 }]}>
            {asParts
              ? photos.length > MAX_PARTS
                ? `Uzun Z raporu: ilk ${MAX_PARTS} fotoğraf parça olarak tek Z raporu olur, diğerleri ayrı gider.`
                : `Uzun Z raporu: ${photos.length} fotoğraf sırasıyla parça 1–${photos.length} olarak tek Z raporu olur.`
              : 'Uzun Z raporu mu? Fotoğrafları tek Z raporu olarak birleştir.'}
          </Text>
        </Pressable>
      ) : null}

      {photos.length > 1 ? (
        <ScrollView horizontal contentContainerStyle={{ gap: 6, paddingHorizontal: 24, paddingTop: 10 }} showsHorizontalScrollIndicator={false}>
          {photos.map((p, i) => (
            <Pressable
              key={`${p.uri}-${i}`}
              accessibilityRole="button"
              accessibilityLabel={`${i + 1}. fotoğrafı çıkar`}
              onPress={() => setPhotos((list) => list.filter((_, j) => j !== i))}
              style={styles.mini}>
              <Image source={{ uri: p.uri }} style={StyleSheet.absoluteFill} contentFit="cover" />
              {partsOn && i < MAX_PARTS ? (
                <View testID={`capture-part-${i + 1}`} style={[styles.partNo, { backgroundColor: c.brand }]}>
                  <Text style={[styles.partNoText, { color: c.onBrand }]}>Parça {i + 1}</Text>
                </View>
              ) : null}
              <View style={styles.miniX}>
                <Ikon ad="kapat" boyut={16} renk={BEYAZ} />
              </View>
            </Pressable>
          ))}
        </ScrollView>
      ) : null}

      <View style={styles.bar}>
        <Pressable testID="capture-gallery" accessibilityRole="button" accessibilityLabel="Galeriden seç" onPress={pick} style={styles.side}>
          <View style={styles.gallery}>{last ? <Image source={{ uri: last.uri }} style={StyleSheet.absoluteFill} contentFit="cover" /> : <Ikon ad="klasor" renk={BEYAZ} />}</View>
          <Text style={[styles.sideText, { color: BEYAZ }]}>Galeri</Text>
        </Pressable>
        <Pressable
          testID="capture-shutter"
          accessibilityRole="button"
          accessibilityLabel={photos.length ? 'Bir tane daha çek' : 'Kamerayla çek'}
          onPress={shoot}
          style={({ pressed }) => [
            styles.shutter,
            { backgroundColor: BEYAZ, borderColor: BEYAZ, boxShadow: `inset 0 0 0 4px ${KOYU}`, opacity: pressed ? 0.85 : 1 },
          ]}
        />
        {photos.length ? (
          <Pressable
            testID="capture-send"
            accessibilityRole="button"
            accessibilityLabel={`${photos.length} fotoğrafı gönder`}
            accessibilityState={{ busy: saving, disabled: saving }}
            disabled={saving}
            onPress={send}
            style={styles.side}>
            <View style={[styles.send, { backgroundColor: c.brand }]}>
              <Ikon ad="onayla" renk={c.onBrand} />
            </View>
            <Text style={[styles.sideText, { color: BEYAZ }]}>Gönder ({photos.length})</Text>
          </Pressable>
        ) : (
          <View style={styles.side} accessible accessibilityLabel="Seri çekim: çektikçe fotoğraflar birikir">
            <View style={[styles.serial, { backgroundColor: c.brand }]}>
              <Text style={[metin.xs, { color: c.onBrand, fontFamily: 'Inter_700Bold' }]}>0</Text>
            </View>
            <Text style={[styles.sideText, { color: BEYAZ }]}>Seri</Text>
          </View>
        )}
      </View>
    </SafeAreaView>
  );
}

const cornerStyle = StyleSheet.create({
  tl: { left: 0, top: 0, borderTopWidth: 3, borderLeftWidth: 3, borderTopLeftRadius: 10 },
  tr: { right: 0, top: 0, borderTopWidth: 3, borderRightWidth: 3, borderTopRightRadius: 10 },
  bl: { left: 0, bottom: 0, borderBottomWidth: 3, borderLeftWidth: 3, borderBottomLeftRadius: 10 },
  br: { right: 0, bottom: 0, borderBottomWidth: 3, borderRightWidth: 3, borderBottomRightRadius: 10 },
});

const styles = StyleSheet.create({
  hd: { flexDirection: 'row', alignItems: 'center', gap: 10, paddingHorizontal: 16, paddingTop: 8 },
  close: { width: DOKUNMA, height: DOKUNMA, borderRadius: 12, backgroundColor: 'rgba(255,255,255,0.12)', alignItems: 'center', justifyContent: 'center' },
  kinds: { flexDirection: 'row', gap: 2, padding: 3, borderRadius: 10, backgroundColor: 'rgba(255,255,255,0.12)' },
  kind: { minHeight: 32, paddingHorizontal: 10, borderRadius: 8, justifyContent: 'center' },
  kindText: { fontFamily: 'Inter_600SemiBold', fontSize: 12 },
  view: { flex: 1, margin: 28, marginTop: 18, borderRadius: 12, overflow: 'hidden', alignItems: 'center', justifyContent: 'center' },
  guide: { alignItems: 'center', gap: 12, paddingHorizontal: 40 },
  corner: { position: 'absolute', width: 34, height: 34, margin: 2 },
  badge: { position: 'absolute', bottom: 14, borderRadius: 999, paddingHorizontal: 12, paddingVertical: 5 },
  mini: { width: 40, height: 52, borderRadius: 6, overflow: 'hidden' },
  parts: { flexDirection: 'row', alignItems: 'center', gap: 10, marginHorizontal: 24, marginTop: 10, minHeight: DOKUNMA, paddingHorizontal: 12, borderRadius: 10, borderWidth: 1 },
  check: { width: 22, height: 22, borderRadius: 6, borderWidth: 2, alignItems: 'center', justifyContent: 'center' },
  partNo: { position: 'absolute', left: 0, right: 0, bottom: 0, alignItems: 'center' },
  partNoText: { fontFamily: 'Inter_600SemiBold', fontSize: 8 },
  miniX: { position: 'absolute', top: 0, right: 0, backgroundColor: 'rgba(0,0,0,0.5)', borderBottomLeftRadius: 6 },
  bar: { flexDirection: 'row', alignItems: 'center', justifyContent: 'space-between', paddingHorizontal: 40, paddingTop: 20, paddingBottom: 20 },
  side: { alignItems: 'center', gap: 4, minWidth: 64, minHeight: DOKUNMA },
  sideText: { fontFamily: 'Inter_500Medium', fontSize: 11 },
  gallery: { width: 44, height: 44, borderRadius: 10, backgroundColor: '#333', overflow: 'hidden', alignItems: 'center', justifyContent: 'center' },
  shutter: { width: 76, height: 76, borderRadius: 38, borderWidth: 4 },
  send: { width: 44, height: 44, borderRadius: 22, alignItems: 'center', justifyContent: 'center' },
  serial: { minWidth: 28, height: 22, borderRadius: 11, paddingHorizontal: 8, alignItems: 'center', justifyContent: 'center', marginVertical: 11 },
});

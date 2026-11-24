import { Ikon } from '@hakan/mobile-core/ui/ikon';
import { DOKUNMA } from '@hakan/mobile-core/ui/layout';
import { metin } from '@hakan/mobile-core/ui/parcalar';
import { useTheme } from '@hakan/mobile-core/ui/theme';
import { colors as temaRenkleri } from '@hakan/mobile-core/ui/tokens';
import { CameraView, useCameraPermissions } from 'expo-camera';
import { Image } from 'expo-image';
import * as ImagePicker from 'expo-image-picker';
import { router, useLocalSearchParams } from 'expo-router';
import { useRef, useState } from 'react';
import { Linking, Pressable, ScrollView, StyleSheet, Text, View, type GestureResponderEvent } from 'react-native';
import { SafeAreaView } from 'react-native-safe-area-context';

import { kindLabels } from '@/features/receipts/fields';
import { FisOnizleme } from '@/features/receipts/fis-onizleme';
import { useReceipts, type CapturedPhoto } from '@/features/receipts/provider';
import { MAX_PARTS, type ReceiptKind } from '@/features/receipts/queue';
import { activeStep, applyStep, hasUltraWideLens, pinchDistance, pinchZoom, ULTRA_WIDE, zoomSteps } from '@/features/receipts/zoom';

const KINDS: ReceiptKind[] = ['expense_receipt', 'fuel_receipt', 'z_report'];
const KISA: Record<ReceiptKind, string> = { expense_receipt: 'Gider fişi', fuel_receipt: 'Akaryakıt', z_report: 'Z raporu' };
const isKind = (v: unknown): v is ReceiptKind => typeof v === 'string' && (KINDS as string[]).includes(v);

/** Kamera ekranı her temada koyu (tasarım: MukellefMobil · 2). */

const HATA_KAMERA = 'Kamera açılamadı. Deklanşöre basınca telefonun kendi kamerası açılır; galeriden de seçebilirsiniz.';
const HATA_CEKIM = 'Fotoğraf çekilemedi. Bir daha deneyin ya da galeriden seçin.';
const HATA_GALERI = 'Galeri açılamadı. Bir daha deneyin.';
const HATA_GONDER = 'Fotoğraflar gönderilemedi. Bir daha deneyin; fotoğraflar bu ekranda duruyor.';

/**
 * Fiş çek (MukellefMobil · 2): fiş türü seçici, canlı kamera vizörü, kenar
 * kılavuzları, büyük deklanşör, galeri ve seri çekim sayacı. Oğuz'un geri
 * bildirimi (Easytrade'deki akış): iki parmakla yakınlaştırma, 0,5× (yalnız
 * ultra geniş mercekli cihazda) / 1× / 2× düğmeleri ve flaş; gider ve
 * akaryakıt fişinde tek fotoğraf çekilince fiş hemen gönderilir, okunur ve
 * önizlemede düzeltilip "Gönder"le onaylanır (FisOnizleme). Z raporu ve
 * galeriden birden çok fotoğraf eskisi gibi birikir ve birlikte gider. Vizöre ya da
 * deklanşöre dokununca uygulamanın içindeki kamera çeker (expo-camera); arka
 * arkaya çekilen fotoğraflar seri olarak birikir. Kamera açılamazsa deklanşör
 * telefonun kendi kamerasını açar (kırpma ile). İzin yoksa ekranda uyarı ve
 * izin/ayarlar düğmesi; her hata ekranda Türkçe yazılır.
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
  const [permission, requestPermission] = useCameraPermissions();
  const cameraRef = useRef<CameraView>(null);
  const [cameraReady, setCameraReady] = useState(false);
  const [cameraFailed, setCameraFailed] = useState(false);
  const [shooting, setShooting] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [saving, setSaving] = useState(false);
  // Uzun Z raporu birkaç fotoğrafta: parça 1/2/3 tek Z raporu olarak gider.
  const [asParts, setAsParts] = useState(false);
  // Fiş önizlemesi: çekilen fişin kuyruk kimliği; gönderince kamera geri gelir.
  const [previewId, setPreviewId] = useState<string | null>(null);
  const [sent, setSent] = useState<string | null>(null);
  // Yakınlaştırma (expo-camera zoom 0–1), mercek (iOS ultra geniş) ve flaş.
  const [zoom, setZoom] = useState(0);
  const [lens, setLens] = useState<string | undefined>(undefined);
  const [ultraWide, setUltraWide] = useState(false);
  const [torch, setTorch] = useState(false);
  const pinch = useRef<{ zoom: number; dist: number } | null>(null);
  const pinchedAt = useRef(0);
  const partsOn = kind === 'z_report' && asParts && photos.length > 1;
  const last = photos[photos.length - 1];

  const granted = permission?.granted === true;
  // İzin hiç sorulmadı (ya da yeniden sorulabilir) / kalıcı reddedildi.
  const askable = permission != null && !granted && permission.canAskAgain;
  const blocked = permission != null && !granted && !permission.canAskAgain;
  const live = granted && !cameraFailed;

  async function askPermission() {
    setError(null);
    try {
      await requestPermission();
    } catch {
      setError(HATA_KAMERA);
    }
  }

  /**
   * A new shot: a single receipt photo (not a Z report, nothing collected
   * yet) is sent at once and read in the preview; otherwise it is collected.
   */
  async function addShots(list: CapturedPhoto[]) {
    if (!list.length) return;
    if (kind === 'z_report' || list.length > 1 || photos.length > 0) {
      setPhotos((p) => [...p, ...list]);
      return;
    }
    try {
      const [id] = await enqueue(kind, list);
      if (!id) throw new Error('not queued');
      setCameraReady(false);
      setPreviewId(id);
    } catch {
      setPhotos((p) => [...p, ...list]);
      setError(HATA_GONDER);
    }
  }

  // Kamera açılamadığında yedek yol: telefonun kendi kamerası (kırpma ile).
  async function shootWithSystemCamera() {
    const res = await ImagePicker.launchCameraAsync({ mediaTypes: ['images'], allowsEditing: true, quality: 0.7 });
    if (!res.canceled) await addShots(res.assets.map((a) => ({ uri: a.uri, mimeType: a.mimeType })));
  }

  // İki parmakla yakınlaştırma (react-native-gesture-handler yok: dokunma olayları).
  function onTouchStart(e: GestureResponderEvent) {
    const d = pinchDistance(e.nativeEvent.touches);
    if (d != null) pinch.current = { zoom: lens === ULTRA_WIDE ? 0 : zoom, dist: d };
  }
  function onTouchMove(e: GestureResponderEvent) {
    const d = pinchDistance(e.nativeEvent.touches);
    if (d == null || !pinch.current) return;
    pinchedAt.current = Date.now();
    const z = pinchZoom(pinch.current.zoom, pinch.current.dist, d);
    if (lens === ULTRA_WIDE) {
      if (z <= 0) return;
      setLens(undefined);
    }
    setZoom(z);
  }
  function onTouchEnd(e: GestureResponderEvent) {
    if (e.nativeEvent.touches.length < 2) pinch.current = null;
  }
  function pickStep(factor: number) {
    const next = applyStep(factor, ultraWide);
    setLens(next.lens);
    setZoom(next.zoom);
  }

  async function shoot() {
    if (shooting) return;
    // A pinch ends with a lift: not a shot.
    if (Date.now() - pinchedAt.current < 600) return;
    setError(null);
    setSent(null);
    if (!granted) {
      if (blocked) return;
      await askPermission();
      return;
    }
    setShooting(true);
    try {
      if (cameraFailed) {
        await shootWithSystemCamera();
        return;
      }
      if (!cameraRef.current || !cameraReady) {
        setError('Kamera hazırlanıyor, bir saniye sonra yeniden deneyin.');
        return;
      }
      const pic = await cameraRef.current.takePictureAsync({ quality: 0.7 });
      if (!pic?.uri) throw new Error('no picture');
      await addShots([{ uri: pic.uri, mimeType: 'image/jpeg' }]);
    } catch {
      setError(HATA_CEKIM);
    } finally {
      setShooting(false);
    }
  }

  async function pick() {
    setError(null);
    setSent(null);
    try {
      const res = await ImagePicker.launchImageLibraryAsync({
        mediaTypes: ['images'],
        allowsMultipleSelection: true,
        selectionLimit: 20,
        quality: 0.7,
      });
      if (!res.canceled) await addShots(res.assets.map((a) => ({ uri: a.uri, mimeType: a.mimeType })));
    } catch {
      setError(HATA_GALERI);
    }
  }

  async function send() {
    setError(null);
    setSaving(true);
    try {
      await enqueue(kind, photos, partsOn);
      router.replace('/receipts');
    } catch {
      setError(HATA_GONDER);
    } finally {
      setSaving(false);
    }
  }

  if (previewId) {
    return (
      <FisOnizleme
        id={previewId}
        onDone={(n) => {
          setPreviewId(null);
          setSent(n ? `Fiş ve ${n} düzeltme müşavirinize gönderildi. Sıradaki fişi çekebilirsiniz.` : 'Fiş müşavirinize gönderildi. Sıradaki fişi çekebilirsiniz.');
        }}
      />
    );
  }

  const steps = zoomSteps(ultraWide);
  const active = activeStep(zoom, lens === ULTRA_WIDE);

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

      <Pressable
        testID="capture-viewfinder"
        accessibilityRole="button"
        accessibilityLabel={live ? 'Vizör: dokununca fotoğraf çekilir' : 'Vizör'}
        disabled={!live || shooting}
        onPress={shoot}
        onTouchStart={live ? onTouchStart : undefined}
        onTouchMove={live ? onTouchMove : undefined}
        onTouchEnd={live ? onTouchEnd : undefined}
        style={[styles.view, { backgroundColor: kamera.surface3 }]}>
        {live ? (
          <CameraView
            ref={cameraRef}
            testID="capture-camera"
            style={StyleSheet.absoluteFill}
            facing="back"
            zoom={zoom}
            enableTorch={torch}
            selectedLens={lens}
            onAvailableLensesChanged={(e) => setUltraWide(hasUltraWideLens(e.lenses))}
            animateShutter
            onCameraReady={() => setCameraReady(true)}
            onMountError={() => {
              setCameraFailed(true);
              setError(HATA_KAMERA);
            }}
          />
        ) : last ? (
          <Image source={{ uri: last.uri }} style={StyleSheet.absoluteFill} contentFit="contain" accessibilityLabel={`Son çekilen fotoğraf, toplam ${photos.length}`} />
        ) : blocked ? (
          <View testID="capture-permission-denied" style={styles.guide}>
            <Ikon ad="fisCek" boyut={28} renk="rgba(255,255,255,0.6)" />
            <Text style={[metin.sm, { color: BEYAZ, textAlign: 'center' }]}>
              {"Kamera izni kapalı. Fişi çekmek için telefonun ayarlarından Hakan'a kamera izni verin; isterseniz galeriden de seçebilirsiniz."}
            </Text>
            <Pressable
              testID="capture-open-settings"
              accessibilityRole="button"
              onPress={() => {
                Linking.openSettings().catch(() => setError('Ayarlar açılamadı. Telefonun Ayarlar uygulamasından Hakan > Kamera iznini açın.'));
              }}
              style={[styles.action, { backgroundColor: BEYAZ }]}>
              <Text style={[styles.actionText, { color: KOYU }]}>Ayarları aç</Text>
            </Pressable>
          </View>
        ) : askable ? (
          <View testID="capture-permission-ask" style={styles.guide}>
            <Ikon ad="fisCek" boyut={28} renk="rgba(255,255,255,0.6)" />
            <Text style={[metin.sm, { color: BEYAZ, textAlign: 'center' }]}>
              Fişi buradan çekebilmek için kamera izni gerekiyor.
            </Text>
            <Pressable
              testID="capture-permission-request"
              accessibilityRole="button"
              onPress={askPermission}
              style={[styles.action, { backgroundColor: BEYAZ }]}>
              <Text style={[styles.actionText, { color: KOYU }]}>Kamera iznini ver</Text>
            </Pressable>
          </View>
        ) : (
          <View style={styles.guide}>
            <Ikon ad="fisCek" boyut={28} renk="rgba(255,255,255,0.6)" />
            <Text style={[metin.sm, { color: 'rgba(255,255,255,0.75)', textAlign: 'center' }]}>
              {cameraFailed
                ? 'Kamera açılamadı. Deklanşöre basınca telefonun kendi kamerası açılır.'
                : kind === 'z_report'
                  ? 'Gün sonu Z raporunun tamamı kadrajda olsun.'
                  : 'Fişi düz bir zemine koyun; toplam, tarih ve VKN okunur olsun.'}
            </Text>
          </View>
        )}
        {live ? (
          <View pointerEvents="none" style={styles.hint}>
            <Text style={[metin.xs, { color: BEYAZ, textAlign: 'center' }]}>
              {kind === 'z_report' ? 'Z raporunun tamamı kadrajda olsun; dokununca çekilir.' : 'Fiş kadrajda olsun; toplam, tarih ve VKN okunsun. İki parmakla yakınlaştırın, dokununca çekilir.'}
            </Text>
          </View>
        ) : null}
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
      </Pressable>

      {live ? (
        <View style={styles.zoom}>
          <View style={styles.zoomSteps} accessibilityRole="radiogroup" accessibilityLabel="Yakınlaştırma; iki parmakla da yakınlaştırabilirsiniz">
            {steps.map((s) => {
              const on = active === s.factor;
              return (
                <Pressable
                  key={s.id}
                  testID={`capture-zoom-${s.id}`}
                  accessibilityRole="radio"
                  accessibilityLabel={`${s.label} yakınlaştırma`}
                  accessibilityState={{ selected: on }}
                  onPress={() => pickStep(s.factor)}
                  hitSlop={6}
                  style={[styles.zoomStep, { backgroundColor: on ? BEYAZ : 'rgba(255,255,255,0.12)' }]}>
                  <Text style={[styles.kindText, { color: on ? KOYU : BEYAZ }]}>{s.label}</Text>
                </Pressable>
              );
            })}
          </View>
          <Pressable
            testID="capture-flash"
            accessibilityRole="switch"
            accessibilityLabel="Flaş"
            accessibilityState={{ checked: torch }}
            onPress={() => setTorch((v) => !v)}
            hitSlop={6}
            style={[styles.zoomStep, { backgroundColor: torch ? BEYAZ : 'rgba(255,255,255,0.12)' }]}>
            <Text style={[styles.kindText, { color: torch ? KOYU : BEYAZ }]}>{torch ? 'Flaş açık' : 'Flaş kapalı'}</Text>
          </Pressable>
        </View>
      ) : null}

      {sent ? (
        <Text testID="capture-sent" accessibilityRole="alert" style={[metin.xs, styles.error, { color: BEYAZ }]}>
          {sent}
        </Text>
      ) : null}

      {error ? (
        <Text testID="capture-error" accessibilityRole="alert" style={[metin.xs, styles.error, { color: BEYAZ }]}>
          {error}
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
            {asParts ? <Ikon ad="onayla" boyut={16} renk={c.onBrand} /> : null}
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
          accessibilityState={{ busy: shooting, disabled: shooting || blocked }}
          disabled={shooting || blocked}
          onPress={shoot}
          style={({ pressed }) => [
            styles.shutter,
            { backgroundColor: BEYAZ, borderColor: BEYAZ, boxShadow: `inset 0 0 0 4px ${KOYU}`, opacity: pressed || shooting || blocked ? 0.5 : 1 },
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
  hint: { position: 'absolute', top: 14, left: 16, right: 16, alignItems: 'center' },
  zoom: { flexDirection: 'row', alignItems: 'center', justifyContent: 'center', gap: 16, marginTop: -8, marginBottom: 8 },
  zoomSteps: { flexDirection: 'row', gap: 8 },
  zoomStep: { minWidth: 44, minHeight: 32, paddingHorizontal: 10, borderRadius: 16, alignItems: 'center', justifyContent: 'center' },
  action: { minHeight: DOKUNMA, paddingHorizontal: 18, borderRadius: 10, alignItems: 'center', justifyContent: 'center' },
  actionText: { fontFamily: 'Inter_600SemiBold', fontSize: 14 },
  error: { textAlign: 'center', paddingHorizontal: 24 },
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

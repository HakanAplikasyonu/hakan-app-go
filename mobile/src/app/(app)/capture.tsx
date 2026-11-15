import { Image } from 'expo-image';
import * as ImagePicker from 'expo-image-picker';
import { router, useLocalSearchParams } from 'expo-router';
import { useState } from 'react';
import { Linking, Pressable, StyleSheet, View } from 'react-native';

import { Button, Notice, Screen, Text } from '@/components/ui';
import { useColors } from '@/core/theme';
import { kindLabels } from '@/features/receipts/fields';
import { useReceipts, type CapturedPhoto } from '@/features/receipts/provider';
import type { ReceiptKind } from '@/features/receipts/queue';

const KINDS: ReceiptKind[] = ['expense_receipt', 'fuel_receipt', 'z_report'];
const isKind = (v: unknown): v is ReceiptKind => typeof v === 'string' && (KINDS as string[]).includes(v);

/** Fiş / Z raporu çekme: kamera (kırpma ile), arka arkaya çoklu çekim veya galeriden çoklu seçim. */
export default function CaptureScreen() {
  const params = useLocalSearchParams<{ kind?: string }>();
  const { enqueue, online } = useReceipts();
  const c = useColors();
  const [kind, setKind] = useState<ReceiptKind>(isKind(params.kind) ? params.kind : 'expense_receipt');
  const [photos, setPhotos] = useState<CapturedPhoto[]>([]);
  const [denied, setDenied] = useState(false);
  const [saving, setSaving] = useState(false);

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
      await enqueue(kind, photos);
      router.replace('/receipts');
    } finally {
      setSaving(false);
    }
  }

  return (
    <Screen style={{ maxWidth: 640, width: '100%', alignSelf: 'center' }}>
      <View style={styles.segment} accessibilityRole="radiogroup">
        {KINDS.map((k) => {
          const active = k === kind;
          return (
            <Pressable
              key={k}
              accessibilityRole="radio"
              accessibilityState={{ selected: active }}
              onPress={() => setKind(k)}
              style={[styles.segmentItem, { borderColor: active ? c.brand : c.border, backgroundColor: active ? c.surface : 'transparent' }]}>
              <Text style={{ fontWeight: active ? '700' : '500', color: active ? c.brand : c.text, textAlign: 'center' }}>{kindLabels[k]}</Text>
            </Pressable>
          );
        })}
      </View>

      <Text tone="muted">
        {kind === 'z_report'
          ? 'Gün sonu Z raporunun tamamı kadrajda olsun.'
          : 'Fişi düz bir zemine koyun; toplam, tarih ve VKN okunur olsun. Çektikten sonra kenarları kırpabilirsiniz.'}
      </Text>

      {denied ? (
        <Notice>
          Kamera izni verilmedi. Ayarlardan izin verebilir ya da galeriden seçebilirsiniz.{' '}
          <Text tone="brand" onPress={() => Linking.openSettings()}>
            Ayarları aç
          </Text>
        </Notice>
      ) : null}

      <Button title={photos.length ? 'Bir tane daha çek' : 'Kamerayla çek'} onPress={shoot} />
      <Button title="Galeriden seç" variant="secondary" onPress={pick} />

      {photos.length ? (
        <View style={styles.grid}>
          {photos.map((p, i) => (
            <View key={`${p.uri}-${i}`} style={[styles.thumb, { borderColor: c.border }]}>
              <Image source={{ uri: p.uri }} style={StyleSheet.absoluteFill} contentFit="cover" />
              <Pressable
                accessibilityRole="button"
                accessibilityLabel={`${i + 1}. fotoğrafı çıkar`}
                onPress={() => setPhotos((list) => list.filter((_, j) => j !== i))}
                style={[styles.remove, { backgroundColor: c.primary }]}>
                <Text style={{ color: c.primaryText, fontWeight: '700' }}>×</Text>
              </Pressable>
            </View>
          ))}
        </View>
      ) : null}

      <View style={{ flex: 1 }} />
      {photos.length ? (
        <Text variant="caption" tone="muted" style={{ textAlign: 'center' }}>
          {online ? 'Fotoğraflar hemen gönderilir.' : 'Bağlantı yok: fotoğraflar telefonda saklanır, bağlantı gelince gönderilir.'}
        </Text>
      ) : null}
      <Button title={photos.length ? `Gönder (${photos.length})` : 'Gönder'} onPress={send} loading={saving} disabled={!photos.length} />
    </Screen>
  );
}

const styles = StyleSheet.create({
  segment: { flexDirection: 'row', gap: 8 },
  segmentItem: { flex: 1, borderWidth: 1, borderRadius: 12, paddingVertical: 10, paddingHorizontal: 6 },
  grid: { flexDirection: 'row', flexWrap: 'wrap', gap: 10 },
  thumb: { width: 96, height: 128, borderRadius: 12, overflow: 'hidden', borderWidth: StyleSheet.hairlineWidth },
  remove: { position: 'absolute', top: 6, right: 6, width: 26, height: 26, borderRadius: 13, alignItems: 'center', justifyContent: 'center' },
});

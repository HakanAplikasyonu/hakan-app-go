import { KristalYukleyici } from '@hakan/mobile-core/hakan';
import { Image } from 'expo-image';
import { router } from 'expo-router';
import { useMemo, useState } from 'react';
import { Pressable, StyleSheet, TextInput, View } from 'react-native';

import { Button, Card, Field, Notice, Screen, Text } from '@/components/ui';
import { useColors } from '@/core/theme';

import { needsAttention } from './fields';
import { FisUyarilari } from './fis-uyarilari';
import {
  addableRates,
  addRow,
  checkPreview,
  editRow,
  fillTax,
  formatAmount,
  previewCorrections,
  previewFrom,
  removeRow,
  type Preview,
} from './onizleme';
import { useReceipts } from './provider';
import { TeslimKarti } from './teslim-karti';

/**
 * Fiş önizlemesi (Oğuz'un geri bildirimi; Easytrade'deki akış): fotoğraf
 * çekilince fiş hemen gönderilir ve "Okunuyor: tarih, tutar ve KDV" yazar.
 * Okuma bitince satıcı, tarih, toplam ve KDV oranı satırları (her oran için
 * matrah ve KDV) gösterilir; mükellef her alanı düzeltip "Gönder"e basar.
 * Düzeltmeler müşavirin inceleme kuyruğuna "Mükellef düzeltmesi" olarak
 * gider. Bağlantı yoksa fiş telefonda bekler; okunanlar Fişlerim'de çıkar.
 */
export function FisOnizleme({ id, onDone }: { id: string; onDone: (corrections: number) => void }) {
  const { items, online, retry, correctMany } = useReceipts();
  const c = useColors();
  const item = items.find((i) => i.id === id);
  const result = item?.result ?? null;
  const original = useMemo(() => (result ? previewFrom(result.fields) : null), [result]);
  const [draft, setDraft] = useState<Preview | null>(null);
  const [draftFor, setDraftFor] = useState<string | null>(null);
  // Render sırasında eşitleme: okuma gelince taslak bir kez kurulur.
  if (result && original && draftFor !== result.uuid) {
    setDraftFor(result.uuid);
    setDraft(original);
  }

  const photo = item ? (
    <View style={[styles.photo, { borderColor: c.border, backgroundColor: c.surface }]}>
      <Image source={{ uri: item.fileUri }} style={StyleSheet.absoluteFill} contentFit="contain" accessibilityLabel="Fiş fotoğrafı" />
    </View>
  ) : null;
  const tamam = (
    <>
      <Button testID="preview-done" title="Tamam" variant="secondary" onPress={() => onDone(0)} />
      <Button testID="preview-gonderdiklerim" title="Gönderdiklerim" variant="ghost" onPress={() => router.replace('/gonderdiklerim')} />
    </>
  );

  if (!item) {
    return (
      <Screen>
        <Text tone="muted">Fiş bulunamadı. Gönderdiklerim ekranından bakabilirsiniz.</Text>
        {tamam}
      </Screen>
    );
  }

  const waiting = item.status === 'pending' || item.status === 'uploading';
  // #780: the server took the photo (202) and reads it in the background.
  const reading = item.status === 'uploaded' && result?.status === 'processing';
  if ((waiting && online && (item.status === 'uploading' || item.attempts === 0)) || reading) {
    return (
      <Screen>
        <TeslimKarti ids={[item.id]} />
        {photo}
        <View testID="preview-reading" style={styles.reading} accessibilityLiveRegion="polite">
          <KristalYukleyici boyut={32} etiket="Okunuyor: tarih, tutar ve KDV" gecikme={0} />
          <Text variant="heading">Okunuyor: tarih, tutar ve KDV</Text>
          <Text variant="caption" tone="muted" style={{ textAlign: 'center' }}>
            {reading
              ? 'Fiş müşavirinize ulaştı ve okunuyor; okunan bilgileri birazdan burada kontrol edebilirsiniz. Beklemek istemezseniz Tamam deyin, sonuç Gönderdiklerim ekranında görünür.'
              : 'Fiş gönderiliyor ve okunuyor; okunan bilgileri birazdan burada kontrol edebilirsiniz.'}
          </Text>
        </View>
        {reading ? tamam : null}
      </Screen>
    );
  }
  if (waiting || item.status === 'blocked') {
    return (
      <Screen>
        <TeslimKarti ids={[item.id]} />
        {photo}
        <Notice tone="info">
          {item.status === 'blocked'
            ? `Fiş telefonda güvende ama şu an gönderilemiyor${item.lastError ? `: ${item.lastError}` : '.'} Gönderim açılınca kendiliğinden gider.`
            : online
              ? 'Fiş şu an gönderilemedi; telefonda bekliyor ve kendiliğinden yeniden denenecek. Okunan bilgileri Gönderdiklerim ekranında kontrol edebilirsiniz.'
              : 'Bağlantı yok. Fiş telefonda bekliyor; bağlantı gelince gönderilir ve okunur. Okunan bilgileri Gönderdiklerim ekranında kontrol edebilirsiniz.'}
        </Notice>
        {tamam}
      </Screen>
    );
  }
  if (item.status === 'failed') {
    return (
      <Screen>
        <TeslimKarti ids={[item.id]} />
        {photo}
        <Notice>{item.lastError ?? 'Fiş gönderilemedi.'} Bir daha deneyin ya da Gönderdiklerim ekranından silin.</Notice>
        <Button testID="preview-retry" title="Tekrar dene" onPress={() => retry(item.id)} />
        {tamam}
      </Screen>
    );
  }
  if (!result || result.status !== 'draft' || !draft || !original) {
    return (
      <Screen>
        <TeslimKarti ids={[item.id]} />
        {photo}
        <Notice tone={result?.status === 'duplicate' ? 'info' : 'danger'}>
          {result?.status === 'duplicate'
            ? 'Bu fiş daha önce gönderilmiş; yeni kayıt açılmadı.'
            : result?.status === 'failed'
              ? 'Fiş okunamadı. Daha net, düz ve ışıklı bir fotoğrafla yeniden çekin.'
              : 'Fiş müşavirinize ulaştı. Okunan bilgileri Gönderdiklerim ekranında görebilirsiniz.'}
        </Notice>
        {tamam}
      </Screen>
    );
  }

  const check = checkPreview(draft);
  const errors = check.errors;
  const attention = (key: string) => {
    const f = result.fields[key];
    return f ? needsAttention(f) : false;
  };
  const set = (patch: Partial<Preview>) => setDraft({ ...draft, ...patch });
  const send = () => {
    const corrections = previewCorrections(result.fields, original, draft);
    correctMany(item.id, corrections);
    onDone(corrections.length);
  };
  const unsure = (key: string) =>
    attention(key) ? (
      <Text variant="caption" style={{ color: c.warning }}>
        Bundan emin olunamadı; fişle karşılaştırın.
      </Text>
    ) : null;

  return (
    <Screen>
      <TeslimKarti ids={[item.id]} />
      <View testID="preview-form" style={{ gap: 4 }}>
        <Text variant="title">Okunan bilgiler</Text>
        <Text variant="caption" tone="muted">
          Okunanları fişle karşılaştırın; yanlışsa düzeltip Gönder’e basın. Düzeltmeleriniz müşavirinizin onayına
          gider.
        </Text>
      </View>
      {photo}
      <FisUyarilari result={result} />

      <View style={{ gap: 6 }}>
        <Field testID="preview-vendor" label="Satıcı" value={draft.vendor} onChangeText={(vendor) => set({ vendor })} error={errors.vendor} autoCapitalize="characters" />
        {unsure('vendor_name')}
      </View>
      <View style={{ gap: 6 }}>
        <Field
          testID="preview-date"
          label="Tarih"
          value={draft.date}
          onChangeText={(date) => set({ date })}
          error={errors.date}
          placeholder="GG.AA.YYYY"
          keyboardType="numbers-and-punctuation"
        />
        {unsure('date')}
      </View>
      <View style={{ gap: 6 }}>
        <Field
          testID="preview-total"
          label="Toplam (KDV dahil, TL)"
          value={draft.total}
          onChangeText={(total) => set({ total })}
          error={errors.total}
          keyboardType="decimal-pad"
        />
        {unsure('total_amount')}
      </View>

      <Card>
        <Text variant="heading">KDV oranları</Text>
        <Text variant="caption" tone="muted">
          Fişteki her KDV oranı ayrı satırdır: matrah (KDV hariç) ve KDV. Fişte olmayan oranı kaldırın, eksik oranı ekleyin.
        </Text>
        {draft.rows.length === 0 ? (
          <Text variant="caption" tone="muted" testID="preview-rows-empty">
            KDV oranı okunamadı. Fişteki oranları aşağıdan ekleyin.
          </Text>
        ) : null}
        {draft.rows.map((row) => (
          <View key={row.rate} testID={`preview-row-${row.rate}`} style={[styles.row, { borderColor: errors[`row_${row.rate}`] ? c.danger : c.border }]}>
            <View style={styles.rowHead}>
              <Text variant="heading">%{row.rate}</Text>
              <View style={{ flex: 1 }} />
              <Pressable
                testID={`preview-fill-${row.rate}`}
                accessibilityRole="button"
                accessibilityLabel={`%${row.rate} KDV'yi matrahtan hesapla`}
                onPress={() => set({ rows: fillTax(draft.rows, row.rate) })}
                style={styles.small}>
                <Text variant="caption" tone="brand">
                  KDV’yi hesapla
                </Text>
              </Pressable>
              <Pressable
                testID={`preview-remove-${row.rate}`}
                accessibilityRole="button"
                accessibilityLabel={`%${row.rate} satırını kaldır`}
                onPress={() => set({ rows: removeRow(draft.rows, row.rate) })}
                style={styles.small}>
                <Text variant="caption" tone="danger">
                  Kaldır
                </Text>
              </Pressable>
            </View>
            <View style={{ flexDirection: 'row', gap: 8 }}>
              <View style={{ flex: 1, gap: 4 }}>
                <Text variant="caption" tone="muted">
                  Matrah
                </Text>
                <TextInput
                  testID={`preview-base-${row.rate}`}
                  value={row.base}
                  onChangeText={(base) => set({ rows: editRow(draft.rows, row.rate, { base }) })}
                  keyboardType="decimal-pad"
                  accessibilityLabel={`%${row.rate} matrah`}
                  style={[styles.input, { color: c.text, borderColor: c.border, backgroundColor: c.background }]}
                />
              </View>
              <View style={{ flex: 1, gap: 4 }}>
                <Text variant="caption" tone="muted">
                  KDV
                </Text>
                <TextInput
                  testID={`preview-tax-${row.rate}`}
                  value={row.tax}
                  onChangeText={(tax) => set({ rows: editRow(draft.rows, row.rate, { tax }) })}
                  keyboardType="decimal-pad"
                  accessibilityLabel={`%${row.rate} KDV`}
                  style={[styles.input, { color: c.text, borderColor: c.border, backgroundColor: c.background }]}
                />
              </View>
            </View>
            {row.estimated ? (
              <Text variant="caption" style={{ color: c.warning }}>
                Bu oranın KDV’si fişten okunamadı; tutardan hesaplandı. Fişle karşılaştırın.
              </Text>
            ) : null}
            {errors[`row_${row.rate}`] ? (
              <Text variant="caption" tone="danger">
                {errors[`row_${row.rate}`]}
              </Text>
            ) : check.notes[row.rate] ? (
              <Text variant="caption" style={{ color: c.warning }}>
                {check.notes[row.rate]}
              </Text>
            ) : null}
          </View>
        ))}
        {addableRates(draft.rows).length ? (
          <View style={{ flexDirection: 'row', gap: 8, flexWrap: 'wrap' }}>
            {addableRates(draft.rows).map((rate) => (
              <Pressable
                key={rate}
                testID={`preview-add-${rate}`}
                accessibilityRole="button"
                accessibilityLabel={`%${rate} KDV satırı ekle`}
                onPress={() => set({ rows: addRow(draft.rows, rate) })}
                style={[styles.add, { borderColor: c.border }]}>
                <Text variant="caption">+ %{rate}</Text>
              </Pressable>
            ))}
          </View>
        ) : null}
        {check.sum && draft.rows.length ? (
          <Text testID="preview-sum" variant="caption" tone={errors.sum ? 'danger' : 'muted'}>
            {errors.sum ?? `Satırların toplamı ${formatAmount(check.sum.total)} TL (matrah ${formatAmount(check.sum.base)}, KDV ${formatAmount(check.sum.tax)}).`}
          </Text>
        ) : null}
      </Card>

      <Button testID="preview-send" title="Gönder" disabled={Object.keys(errors).length > 0} onPress={send} />
      {Object.keys(errors).length ? (
        <Text variant="caption" tone="danger" style={{ textAlign: 'center' }}>
          Göndermek için kırmızı alanları düzeltin.
        </Text>
      ) : null}
    </Screen>
  );
}

const styles = StyleSheet.create({
  photo: { height: 180, borderRadius: 16, overflow: 'hidden', borderWidth: StyleSheet.hairlineWidth },
  reading: { alignItems: 'center', gap: 10, paddingVertical: 24 },
  row: { borderWidth: 1, borderRadius: 12, padding: 10, gap: 8 },
  rowHead: { flexDirection: 'row', alignItems: 'center', gap: 8 },
  small: { minHeight: 32, paddingHorizontal: 6, justifyContent: 'center' },
  add: { minHeight: 36, paddingHorizontal: 12, borderRadius: 999, borderWidth: 1, justifyContent: 'center' },
  input: { minHeight: 44, borderWidth: 1, borderRadius: 10, paddingHorizontal: 12, fontSize: 16 },
});

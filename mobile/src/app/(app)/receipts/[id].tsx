import { Image } from 'expo-image';
import { router, useLocalSearchParams } from 'expo-router';
import { useState } from 'react';
import { Pressable, StyleSheet, TextInput, View } from 'react-native';

import { Button, Card, Chip, Notice, Screen, Text } from '@/components/ui';
import { useColors } from '@/core/theme';
import { fieldLabel, kindLabels, LOW_CONFIDENCE, needsAttention, orderedFields, statusLabel } from '@/features/receipts/fields';
import { useReceipts } from '@/features/receipts/provider';
import { shotAt, statusTone } from '@/features/receipts/status';

/** Fiş ayrıntısı: fotoğraf, OCR sonucu (emin olunamayan alanlar sarı) ve düzeltme önerisi. */
export default function ReceiptScreen() {
  const { id } = useLocalSearchParams<{ id: string }>();
  const { items, retry, remove, correct } = useReceipts();
  const c = useColors();
  const item = items.find((i) => i.id === id);
  const [editing, setEditing] = useState<string | null>(null);
  const [draft, setDraft] = useState('');

  if (!item) {
    return (
      <Screen>
        <Text tone="muted">Fiş bulunamadı.</Text>
      </Screen>
    );
  }
  const r = item.result;
  const corrections = new Map(item.corrections.map((x) => [x.field, x.value]));

  return (
    <Screen style={{ maxWidth: 640, width: '100%', alignSelf: 'center' }}>
      <View style={[styles.photo, { borderColor: c.border, backgroundColor: c.surface }]}>
        <Image source={{ uri: item.fileUri }} style={StyleSheet.absoluteFill} contentFit="contain" accessibilityLabel="Fiş fotoğrafı" />
      </View>

      <View style={{ flexDirection: 'row', alignItems: 'center', justifyContent: 'space-between', gap: 8 }}>
        <View style={{ gap: 2, flex: 1 }}>
          <Text variant="heading">{kindLabels[item.kind]}</Text>
          <Text variant="caption" tone="muted">
            Çekildi: {shotAt(item.createdAt)}
          </Text>
        </View>
        <Chip label={statusLabel(item)} tone={statusTone(item)} />
      </View>

      {item.status === 'blocked' ? (
        <Notice tone="info">Fotoğraf telefonda güvende. Mükellef tarafından gönderim açıldığında kendiliğinden gönderilecek.</Notice>
      ) : null}
      {item.status === 'pending' && item.lastError ? <Notice tone="info">{item.lastError} Bağlantı gelince yeniden denenecek.</Notice> : null}
      {item.status === 'failed' ? <Notice>{item.lastError ?? 'Gönderilemedi.'}</Notice> : null}
      {r?.status === 'duplicate' ? <Notice tone="info">Bu fiş daha önce gönderilmiş; yeni kayıt açılmadı.</Notice> : null}
      {r?.status === 'failed' ? <Notice>Fiş okunamadı{r.error ? `: ${r.error}` : ''}. Daha net bir fotoğrafla yeniden çekin.</Notice> : null}
      {r?.plate_check === 'not_registered' ? (
        <Notice>Plaka araç listenizde yok. Müşavirinize bu aracı bildirin.</Notice>
      ) : null}

      {r && Object.keys(r.fields).length ? (
        <Card>
          <Text variant="heading">Okunan bilgiler</Text>
          <Text variant="caption" tone="muted">
            Sarı alanlardan emin olunamadı. Yanlışsa dokunup doğrusunu yazın; müşaviriniz onaylarken görür.
          </Text>
          {orderedFields(r.fields).map(([key, f]) => {
            const low = needsAttention(f);
            const fixed = corrections.get(key);
            return (
              <View key={key} style={[styles.field, { borderColor: low && !fixed ? c.warning : c.border }]}>
                <Pressable
                  accessibilityRole="button"
                  accessibilityLabel={`${fieldLabel(key)}: ${fixed ?? (f.value || f.raw)}. Düzelt`}
                  onPress={() => {
                    setEditing(key);
                    setDraft(fixed ?? f.value ?? f.raw);
                  }}
                  style={{ flexDirection: 'row', alignItems: 'center', gap: 8 }}>
                  <Text variant="caption" tone="muted" style={{ width: 96 }}>
                    {fieldLabel(key)}
                  </Text>
                  <View style={{ flex: 1 }}>
                    <Text style={{ fontVariant: ['tabular-nums'], textDecorationLine: fixed ? 'line-through' : 'none' }} tone={fixed ? 'muted' : 'default'}>
                      {f.value || f.raw || '—'}
                    </Text>
                    {fixed ? <Text style={{ fontVariant: ['tabular-nums'], fontWeight: '600' }}>{fixed}</Text> : null}
                  </View>
                  {low && !fixed ? (
                    <Text variant="caption" style={{ color: c.warning }}>
                      %{Math.round(f.confidence * 100)}
                    </Text>
                  ) : null}
                  {fixed ? <Chip label="Önerildi" tone="brand" /> : null}
                </Pressable>
                {editing === key ? (
                  <View style={{ gap: 8, marginTop: 8 }}>
                    <TextInput
                      value={draft}
                      onChangeText={setDraft}
                      autoFocus
                      style={[styles.input, { color: c.text, borderColor: c.border, backgroundColor: c.background }]}
                      accessibilityLabel={`${fieldLabel(key)} doğru değer`}
                    />
                    <View style={{ flexDirection: 'row', gap: 8 }}>
                      <View style={{ flex: 1 }}>
                        <Button title="Vazgeç" variant="secondary" onPress={() => setEditing(null)} />
                      </View>
                      <View style={{ flex: 1 }}>
                        <Button
                          title="Öner"
                          disabled={!draft.trim()}
                          onPress={() => {
                            correct(item.id, key, draft.trim());
                            setEditing(null);
                          }}
                        />
                      </View>
                    </View>
                  </View>
                ) : null}
              </View>
            );
          })}
          {item.corrections.length ? (
            <Text variant="caption" tone="muted">
              {item.correctionsError
                ? `Düzeltme önerileriniz kabul edilmedi: ${item.correctionsError}`
                : item.correctionsSent
                  ? 'Düzeltme önerileriniz müşavirinize iletildi.'
                  : 'Düzeltme önerileriniz gönderilmeyi bekliyor.'}
            </Text>
          ) : null}
        </Card>
      ) : item.status !== 'uploaded' ? (
        <Text variant="caption" tone="muted">
          Okunan bilgiler fiş gönderilince burada görünür (eşik %{Math.round(LOW_CONFIDENCE * 100)} altı sarı).
        </Text>
      ) : null}

      <View style={{ flex: 1 }} />
      {item.status === 'failed' || item.status === 'blocked' || (item.status === 'pending' && item.attempts > 0) ? (
        <Button title="Şimdi tekrar dene" variant="secondary" onPress={() => retry(item.id)} />
      ) : null}
      {item.status !== 'uploading' ? (
        <Button
          title={item.status === 'uploaded' ? 'Telefondan kaldır' : 'Fişi sil'}
          variant="ghost"
          onPress={() => {
            remove(item.id);
            router.back();
          }}
        />
      ) : null}
    </Screen>
  );
}

const styles = StyleSheet.create({
  photo: { height: 280, borderRadius: 16, overflow: 'hidden', borderWidth: StyleSheet.hairlineWidth },
  field: { borderWidth: 1, borderRadius: 12, padding: 10 },
  input: { minHeight: 44, borderWidth: 1, borderRadius: 10, paddingHorizontal: 12, fontSize: 16 },
});

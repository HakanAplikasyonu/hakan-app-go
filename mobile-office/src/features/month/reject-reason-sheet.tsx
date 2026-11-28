import { useState } from 'react';
import { KeyboardAvoidingView, Modal, Platform, Pressable, ScrollView, StyleSheet, View } from 'react-native';

import { Button, Field, Text } from '@/components/ui';
import { useColors } from '@/core/theme';

import { MAX_REJECT_TEXT, REJECT_REASONS, type RejectReasonCode, type Rejection, rejectionBody } from './reject-reason';

type Props = {
  /** Başlık, ör. "Masrafı reddet". null: kapalı. */
  title: string | null;
  hint?: string;
  busy: boolean;
  onCancel: () => void;
  onSubmit: (reason: Rejection) => void;
};

/**
 * Ret gerekçesi (#780): hazır gerekçeler ve açıklama alanı. Seçilen gerekçe
 * mükellefe gider ve Gönderdiklerim ekranında görünür.
 */
export function RejectReasonSheet(props: Props) {
  // Her açılışta alanlar sıfırdan başlasın.
  return <Sheet key={props.title ?? 'closed'} {...props} />;
}

function Sheet({ title, hint, busy, onCancel, onSubmit }: Props) {
  const c = useColors();
  const [code, setCode] = useState<RejectReasonCode | null>(null);
  const [text, setText] = useState('');
  const [tried, setTried] = useState(false);
  const result = rejectionBody(code, text);
  const error = tried && 'error' in result ? result.error : null;
  return (
    <Modal visible={!!title} transparent animationType="slide" onRequestClose={onCancel}>
      <KeyboardAvoidingView style={{ flex: 1 }} behavior={Platform.OS === 'ios' ? 'padding' : undefined}>
        <Pressable style={styles.backdrop} onPress={onCancel} accessibilityLabel="Kapat" />
        <View style={[styles.sheet, { backgroundColor: c.background, borderColor: c.border }]}>
          <ScrollView contentContainerStyle={{ gap: 12 }} keyboardShouldPersistTaps="handled">
            <Text variant="heading">{title ?? ''}</Text>
            <Text tone="muted">{hint ?? 'Seçtiğiniz gerekçe mükellefe bildirilir ve Gönderdiklerim ekranında görünür.'}</Text>
            <View accessibilityRole="radiogroup" style={{ gap: 8 }}>
              {REJECT_REASONS.map((r) => {
                const on = r.code === code;
                return (
                  <Pressable
                    key={r.code}
                    testID={`reject-reason-${r.code}`}
                    accessibilityRole="radio"
                    accessibilityState={{ checked: on }}
                    onPress={() => setCode(r.code)}
                    style={[styles.option, { borderColor: on ? c.brand : c.border, backgroundColor: c.surface }]}>
                    <View style={[styles.dot, { borderColor: on ? c.brand : c.border, backgroundColor: on ? c.brand : 'transparent' }]} />
                    <Text style={{ flex: 1 }}>{r.label}</Text>
                  </Pressable>
                );
              })}
            </View>
            <Field
              testID="reject-reason-text"
              label={code === 'diger' ? 'Açıklama' : 'Açıklama (isteğe bağlı)'}
              value={text}
              onChangeText={setText}
              placeholder="Mükellefe iletmek istediğiniz not"
              maxLength={MAX_REJECT_TEXT}
              multiline
              style={{ minHeight: 72 }}
              error={error}
            />
            <Button
              testID="reject-reason-confirm"
              title="Reddet"
              onPress={() => {
                setTried(true);
                if ('body' in result) onSubmit(result.body);
              }}
              disabled={!code || busy}
              loading={busy}
            />
            <Button title="Vazgeç" variant="ghost" onPress={onCancel} />
          </ScrollView>
        </View>
      </KeyboardAvoidingView>
    </Modal>
  );
}

const styles = StyleSheet.create({
  backdrop: { flex: 1, backgroundColor: 'rgba(0,0,0,0.35)' },
  sheet: { maxHeight: '85%', padding: 20, borderTopLeftRadius: 20, borderTopRightRadius: 20, borderWidth: StyleSheet.hairlineWidth },
  option: { minHeight: 44, flexDirection: 'row', alignItems: 'center', gap: 10, borderWidth: 1, borderRadius: 12, paddingHorizontal: 12 },
  dot: { width: 16, height: 16, borderRadius: 8, borderWidth: 2 },
});

import { useState } from 'react';
import { KeyboardAvoidingView, Modal, Platform, Pressable, StyleSheet, View } from 'react-native';

import { Button, Field, Text } from '@/components/ui';
import { useColors } from '@/core/theme';

import { isValidAccountCode } from './format';

export type ReasonSheetMode = { kind: 'reject' } | { kind: 'correct'; initialCode: string };

/** Reddet / Düzelt için gerekçe (ve düzeltmede yeni hesap kodu) alır. Gerekçe backend'de zorunlu. */
type Props = {
  mode: ReasonSheetMode | null;
  busy: boolean;
  error: string | null;
  onCancel: () => void;
  onSubmit: (input: { reason: string; accountCode?: string }) => void;
};

export function ReasonSheet(props: Props) {
  // Her açılışta alanlar sıfırdan başlasın: içerik moda göre yeniden kurulur.
  const key = props.mode ? (props.mode.kind === 'correct' ? `correct:${props.mode.initialCode}` : 'reject') : 'closed';
  return <Sheet key={key} {...props} />;
}

function Sheet({ mode, busy, error, onCancel, onSubmit }: Props) {
  const c = useColors();
  const [reason, setReason] = useState('');
  const [code, setCode] = useState(mode?.kind === 'correct' ? mode.initialCode : '');
  const correcting = mode?.kind === 'correct';
  const codeOk = !correcting || isValidAccountCode(code);
  const can = reason.trim().length >= 3 && codeOk && !busy;

  return (
    <Modal visible={!!mode} transparent animationType="slide" onRequestClose={onCancel}>
      <KeyboardAvoidingView style={{ flex: 1 }} behavior={Platform.OS === 'ios' ? 'padding' : undefined}>
        <Pressable style={styles.backdrop} onPress={onCancel} accessibilityLabel="Kapat" />
        <View style={[styles.sheet, { backgroundColor: c.background, borderColor: c.border }]}>
          <Text variant="heading">{correcting ? 'Hesap kodunu düzelt' : 'Öneriyi reddet'}</Text>
          {correcting ? (
            <Field
              label="Hesap kodu"
              value={code}
              onChangeText={setCode}
              autoCapitalize="none"
              autoCorrect={false}
              keyboardType="numbers-and-punctuation"
              error={code && !codeOk ? 'Örn. 770 ya da 770.01' : null}
            />
          ) : null}
          <Field label="Gerekçe" value={reason} onChangeText={setReason} placeholder="Kısa bir açıklama" multiline style={{ minHeight: 72 }} />
          {error ? (
            <Text tone="danger" variant="caption">
              {error}
            </Text>
          ) : null}
          <Button
            title={correcting ? 'Düzelt ve uygula' : 'Reddet'}
            onPress={() => onSubmit({ reason: reason.trim(), accountCode: correcting ? code.trim() : undefined })}
            disabled={!can}
            loading={busy}
          />
          <Button title="Vazgeç" variant="ghost" onPress={onCancel} />
        </View>
      </KeyboardAvoidingView>
    </Modal>
  );
}

const styles = StyleSheet.create({
  backdrop: { flex: 1, backgroundColor: 'rgba(0,0,0,0.35)' },
  sheet: { padding: 20, gap: 12, borderTopLeftRadius: 20, borderTopRightRadius: 20, borderWidth: StyleSheet.hairlineWidth },
});

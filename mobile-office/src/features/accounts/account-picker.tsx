import { useState } from 'react';
import { ActivityIndicator, KeyboardAvoidingView, Modal, Platform, Pressable, ScrollView, StyleSheet, View } from 'react-native';

import { Button, Field, Row, Text } from '@/components/ui';
import { useAuth } from '@/core/auth/auth-provider';
import { useColors } from '@/core/theme';
import { useAccountSearch } from '@/features/month/api';
import { accountQuery } from '@/features/month/format';

import { filterAccounts, initialQuery, manualCode } from './filter';

export type PickedAccount = { code: string; name: string | null };

type Props = {
  engagement: string;
  visible: boolean;
  /** Şu an seçili / önerilen kod; arama bu kodun ana hesabıyla açılır. */
  current: string | null;
  onPick: (a: PickedAccount) => void;
  onClose: () => void;
};

/**
 * Aranabilir hesap planı seçici (onay ekranı, Oğuz'un canlı geri
 * bildirimi): mükellefin hesap planında kod öneki ya da adla arar
 * (`GET .../account-plan/accounts?q=`), yalnız alt hesaplar seçilir. Plan
 * yüklü değilse ya da kod planda yoksa biçimi doğru kod elle seçilebilir.
 */
export function AccountPicker(props: Props) {
  // Her açılışta arama, seçili kodun ana hesabıyla baştan başlar.
  return props.visible ? <Picker key={props.current ?? ''} {...props} /> : null;
}

function Picker({ engagement, visible, current, onPick, onClose }: Props) {
  const c = useColors();
  const { can } = useAuth();
  const canPlan = can('tenant.account_plan.read');
  const [search, setSearch] = useState(initialQuery(current));
  const q = accountQuery(search);
  const accounts = useAccountSearch(engagement, q, canPlan);
  const shown = filterAccounts(accounts.data?.items ?? [], search.trim());
  const manual = manualCode(search, shown);

  return (
    <Modal visible={visible} transparent animationType="slide" onRequestClose={onClose}>
      <KeyboardAvoidingView style={{ flex: 1 }} behavior={Platform.OS === 'ios' ? 'padding' : undefined}>
        <Pressable style={styles.backdrop} onPress={onClose} accessibilityLabel="Kapat" />
        <View style={[styles.sheet, { backgroundColor: c.background, borderColor: c.border }]} testID="account-picker">
          <Text variant="heading">Hesap kodunu seçin</Text>
          {current ? (
            <Text variant="caption" tone="muted">
              Şu anki kod: {current}
            </Text>
          ) : null}
          <Field
            label="Hesap kodu ya da adı"
            value={search}
            onChangeText={setSearch}
            placeholder="Örn. 770 ya da kırtasiye"
            autoCapitalize="none"
            autoCorrect={false}
            autoFocus
            testID="account-picker-search"
          />
          <ScrollView style={{ maxHeight: 320 }} keyboardShouldPersistTaps="handled" contentContainerStyle={{ gap: 6 }}>
            {!q ? (
              <Text variant="caption" tone="muted">
                Aramak için en az 2 karakter yazın.
              </Text>
            ) : null}
            {q && accounts.isFetching ? <ActivityIndicator color={c.brand} /> : null}
            {q && !canPlan ? (
              <Text variant="caption" tone="muted">
                Hesap planını görme yetkiniz yok; kodu elle yazabilirsiniz.
              </Text>
            ) : null}
            {q && canPlan && accounts.data && !shown.length && !manual ? (
              <Text variant="caption" tone="muted">
                Hesap planında eşleşen alt hesap yok.
              </Text>
            ) : null}
            {shown.map((a) => (
              <View key={a.code} testID="account-picker-row">
                <Row
                  title={`${a.code} · ${a.name}`}
                  accessibilityLabel={`${a.code} ${a.name} hesabını seç`}
                  onPress={() => onPick({ code: a.code, name: a.name })}
                />
              </View>
            ))}
            {manual ? (
              <View testID="account-picker-manual">
                <Row title={`${manual} kodunu kullan`} subtitle="Hesap planında yok; elle girilen kod" onPress={() => onPick({ code: manual, name: null })} />
              </View>
            ) : null}
          </ScrollView>
          <Button title="Vazgeç" variant="ghost" onPress={onClose} />
        </View>
      </KeyboardAvoidingView>
    </Modal>
  );
}

const styles = StyleSheet.create({
  backdrop: { flex: 1, backgroundColor: 'rgba(0,0,0,0.35)' },
  sheet: { padding: 20, gap: 12, borderTopLeftRadius: 20, borderTopRightRadius: 20, borderWidth: StyleSheet.hairlineWidth },
});

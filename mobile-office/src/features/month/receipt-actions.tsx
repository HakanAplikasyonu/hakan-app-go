import { useState } from 'react';
import { ActivityIndicator, Alert, Pressable, ScrollView, StyleSheet, View } from 'react-native';

import { Button, Card, Chip, Field, Notice, Row, Text } from '@/components/ui';
import { errorMessage } from '@/core/api/envelope';
import { useAuth } from '@/core/auth/auth-provider';
import { useColors } from '@/core/theme';
import { isValidAccountCode } from '@/features/review/format';

import { type Expense, useAccountSearch, useExpenseCategories, useExpensePosting, useReceiptDecision } from './api';
import { accountQuery, accountSourceLabel } from './format';

/**
 * Bekleyen masrafın hesabı ve kararı (#780): önerilen hesap kodu ve
 * kaynağı; kategori ya da hesap planından hesap seçimi (`PATCH`), sonra
 * Onayla (yevmiye taslağı kurulur) ya da Reddet. Web'deki Masraflar
 * sayfasıyla aynı uçlar.
 */
export function ExpenseActions({ engagement, expense }: { engagement: string; expense: Expense }) {
  const { can } = useAuth();
  const c = useColors();
  const canWrite = can('tenant.receipts.write');
  const canApprove = can('tenant.receipts.approve');
  const canPlan = can('tenant.account_plan.read');
  const posting = useExpensePosting(engagement, expense.uuid);
  const decision = useReceiptDecision(engagement, 'expenses', expense.uuid);
  const categories = useExpenseCategories(engagement, canWrite);
  const [search, setSearch] = useState('');
  const q = accountQuery(search);
  const accounts = useAccountSearch(engagement, q, canWrite && canPlan);
  const [done, setDone] = useState<string | null>(null);

  const pending = expense.approval_status === 'pending';
  const source = accountSourceLabel(expense.account_source);
  const busy = posting.isPending || decision.isPending;
  const leafs = (accounts.data?.items ?? []).filter((a) => a.is_leaf);
  const cats = (categories.data?.items ?? []).filter((x) => x.is_active);
  const typed = search.trim();
  const manual = typed && isValidAccountCode(typed) && !leafs.some((a) => a.code === typed) ? typed : null;

  const setAccount = (account_code: string) =>
    posting.mutate(
      { account_code },
      {
        onSuccess: () => {
          setSearch('');
          setDone(`Hesap ${account_code} olarak değiştirildi.`);
        },
      },
    );
  const setCategory = (category_code: string, name: string) =>
    posting.mutate({ category_code }, { onSuccess: () => setDone(`Kategori "${name}" seçildi; hesabı kategoriden alındı.`) });

  const approve = () => decision.mutate(true, { onSuccess: () => setDone('Masraf onaylandı; yevmiye taslağı oluşturuldu.') });
  const reject = () =>
    Alert.alert('Masrafı reddet', 'Reddedilen masraf beyana ve yevmiyeye girmez.', [
      { text: 'Vazgeç', style: 'cancel' },
      { text: 'Reddet', style: 'destructive', onPress: () => decision.mutate(false, { onSuccess: () => setDone('Masraf reddedildi.') }) },
    ]);

  return (
    <Card>
      <Text variant="heading">Hesap</Text>
      <View style={{ flexDirection: 'row', alignItems: 'center', gap: 8, flexWrap: 'wrap' }}>
        <Text variant="title" testID="expense-account">
          {expense.account_code || '—'}
        </Text>
        {expense.suggested && pending ? <Chip tone="brand" label="Öneri" /> : null}
        {expense.review_fields.includes('account_code') ? <Chip tone="warning" label="Kontrol et" /> : null}
      </View>
      {expense.category_name ? <Text tone="muted">Kategori: {expense.category_name}</Text> : null}
      {source ? (
        <Text variant="caption" tone="muted">
          Kaynak: {source}
        </Text>
      ) : null}
      {expense.journal ? (
        <Text variant="caption" tone="muted">
          Yevmiye fişi {expense.journal.entry_no} ({expense.journal.status === 'draft' ? 'taslak' : expense.journal.status === 'approved' ? 'onaylı' : 'aktarıldı'})
        </Text>
      ) : null}

      {pending && canWrite ? (
        <View style={{ gap: 10, marginTop: 6 }}>
          {cats.length ? (
            <View style={{ gap: 6 }}>
              <Text variant="caption" tone="muted">
                Kategori seçin
              </Text>
              <ScrollView horizontal showsHorizontalScrollIndicator={false} contentContainerStyle={{ gap: 8 }}>
                {cats.map((x) => {
                  const on = x.code === expense.category_code;
                  return (
                    <Pressable
                      key={`${x.scope}-${x.code}`}
                      accessibilityRole="button"
                      accessibilityState={{ selected: on }}
                      accessibilityLabel={`${x.name}, hesap ${x.account_code}`}
                      disabled={busy || on}
                      onPress={() => setCategory(x.code, x.name)}
                      style={[styles.pill, { borderColor: on ? c.brand : c.border, backgroundColor: on ? c.brand : c.surface }]}>
                      <Text variant="caption" style={{ color: on ? c.background : c.text, fontWeight: '600' }}>
                        {x.name} · {x.account_code}
                      </Text>
                    </Pressable>
                  );
                })}
              </ScrollView>
            </View>
          ) : null}
          <Field
            label="Ya da hesap kodu / adı ile arayın"
            value={search}
            onChangeText={setSearch}
            placeholder="Örn. 770 ya da kırtasiye"
            autoCapitalize="none"
            autoCorrect={false}
            testID="expense-account-search"
          />
          {accounts.isFetching ? <ActivityIndicator color={c.brand} /> : null}
          {q && canPlan && accounts.data && !leafs.length && !manual ? (
            <Text variant="caption" tone="muted">
              Hesap planında eşleşen alt hesap yok.
            </Text>
          ) : null}
          {leafs.slice(0, 8).map((a) => (
            <Row key={a.code} title={`${a.code} · ${a.name}`} accessibilityLabel={`${a.code} ${a.name} hesabını seç`} onPress={() => setAccount(a.code)} />
          ))}
          {manual ? <Row title={`${manual} kodunu kullan`} subtitle="Elle girilen hesap kodu" onPress={() => setAccount(manual)} /> : null}
        </View>
      ) : null}

      {done ? <Notice tone="info">{done}</Notice> : null}
      {posting.error ? <Notice>{errorMessage(posting.error)}</Notice> : null}
      {decision.error ? <Notice>{errorMessage(decision.error)}</Notice> : null}

      {pending && canApprove ? (
        <View style={{ gap: 10, marginTop: 6 }}>
          <Button testID="expense-approve" title="Onayla" onPress={approve} loading={decision.isPending} disabled={busy} />
          <Button title="Reddet" variant="ghost" onPress={reject} disabled={busy} />
        </View>
      ) : null}
      {pending && !canApprove ? (
        <Text variant="caption" tone="muted">
          Masraf onaylama yetkiniz yok.
        </Text>
      ) : null}
    </Card>
  );
}

/** Bekleyen Z raporunun kararı (#780): Onayla (yevmiye taslağı) ya da Reddet. */
export function ZReportActions({ engagement, uuid }: { engagement: string; uuid: string }) {
  const { can } = useAuth();
  const decision = useReceiptDecision(engagement, 'zreports', uuid);
  const [done, setDone] = useState<string | null>(null);
  if (!can('tenant.receipts.approve')) {
    return (
      <Text variant="caption" tone="muted">
        Z raporu onaylama yetkiniz yok.
      </Text>
    );
  }
  const reject = () =>
    Alert.alert('Z raporunu reddet', 'Reddedilen Z raporu beyana ve yevmiyeye girmez.', [
      { text: 'Vazgeç', style: 'cancel' },
      { text: 'Reddet', style: 'destructive', onPress: () => decision.mutate(false, { onSuccess: () => setDone('Z raporu reddedildi.') }) },
    ]);
  return (
    <View style={{ gap: 10 }}>
      {done ? <Notice tone="info">{done}</Notice> : null}
      {decision.error ? <Notice>{errorMessage(decision.error)}</Notice> : null}
      <Button
        testID="zreport-approve"
        title="Onayla"
        onPress={() => decision.mutate(true, { onSuccess: () => setDone('Z raporu onaylandı; yevmiye taslağı oluşturuldu.') })}
        loading={decision.isPending}
        disabled={decision.isPending}
      />
      <Button title="Reddet" variant="ghost" onPress={reject} disabled={decision.isPending} />
    </View>
  );
}

const styles = StyleSheet.create({
  pill: { minHeight: 36, borderRadius: 999, borderWidth: 1, paddingHorizontal: 12, justifyContent: 'center' },
});

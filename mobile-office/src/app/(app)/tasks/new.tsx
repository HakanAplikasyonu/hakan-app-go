import { Ikon } from '@hakan/mobile-core/ui/ikon';
import { DOKUNMA } from '@hakan/mobile-core/ui/layout';
import { Kart, metin } from '@hakan/mobile-core/ui/parcalar';
import { useTheme } from '@hakan/mobile-core/ui/theme';
import { useLocalSearchParams, useRouter } from 'expo-router';
import { useRef, useState, type ComponentRef } from 'react';
import { KeyboardAvoidingView, Platform, Pressable, ScrollView, StyleSheet, Text, TextInput, View } from 'react-native';
import { SafeAreaView } from 'react-native-safe-area-context';

import { Notice } from '@/components/ui';
import { errorMessage } from '@/core/api/envelope';
import { useCreateRequest } from '@/features/doctasks/api';
import { channelLabel, channelOrder, defaultChannels, remindLabel, remindOptions, toggleChannel, type Channel } from '@/features/doctasks/channels';
import { commonRequests, dayLabel, daysUntilDate, dueChoices, dueDateFromDays, validateRequest } from '@/features/doctasks/format';

type Due = number | 'tarih';

/**
 * Belge iste (MusavirMobil · 4): hazır belge çipleri (ya da "Diğer"), not,
 * son gün (3 / 7 / 14 / tarih), gönderim kanalları (bildirim + SMS / e-posta, #780).
 */
export default function NewRequestScreen() {
  const { engagement, title: taxpayer } = useLocalSearchParams<{ engagement: string; title?: string }>();
  const router = useRouter();
  const { colors: c } = useTheme();
  const create = useCreateRequest(engagement);
  const titleRef = useRef<ComponentRef<typeof TextInput>>(null);
  const [channels, setChannels] = useState<Channel[]>(defaultChannels);
  const [title, setTitle] = useState('');
  const [note, setNote] = useState('');
  const [due, setDue] = useState<Due>(7);
  const [dateText, setDateText] = useState('');
  const [touched, setTouched] = useState(false);
  const [remind, setRemind] = useState<number | null>(null);
  const days = due === 'tarih' ? daysUntilDate(dateText) : due;
  const reminds = remindOptions(days);
  const remindDays = remind !== null && reminds.includes(remind) ? remind : null;
  const invalid = due === 'tarih' && days === null ? 'Son günü GG.AA.YYYY biçiminde, 1–90 gün içinde yazın.' : validateRequest({ title, note, dueDays: days ?? 0 });

  async function submit() {
    setTouched(true);
    if (invalid || days === null) return;
    try {
      // Kanallar ve hatırlatma istekle birlikte gider; sunucu her kanaldan gönderir.
      const t = await create.mutateAsync({
        title: title.trim(),
        note: note.trim() || undefined,
        due_date: dueDateFromDays(days),
        channels,
        ...(remindDays !== null ? { remind_days_before: remindDays } : {}),
      });
      router.replace({ pathname: '/tasks/[uuid]', params: { uuid: t.uuid, engagement } });
    } catch {
      // create.error gösterilir
    }
  }

  const input = [styles.input, metin.govde, { color: c.text, backgroundColor: c.surface, borderColor: c.line }];
  return (
    <SafeAreaView style={{ flex: 1, backgroundColor: c.bg }} edges={['top', 'left', 'right', 'bottom']}>
      <View style={styles.hd}>
        <Pressable accessibilityRole="button" accessibilityLabel="Kapat" onPress={() => router.back()} style={styles.icon}>
          <Ikon ad="kapat" renk={c.text} />
        </Pressable>
        <Text accessibilityRole="header" style={[metin.govde, { fontFamily: 'Inter_700Bold', color: c.text, flex: 1 }]}>
          Belge iste
        </Text>
        {taxpayer ? (
          <Text style={[metin.sm, { color: c.text3, maxWidth: '50%' }]} numberOfLines={1}>
            {taxpayer}
          </Text>
        ) : null}
      </View>
      <KeyboardAvoidingView style={{ flex: 1 }} behavior={Platform.OS === 'ios' ? 'padding' : undefined}>
        <ScrollView contentContainerStyle={styles.body} keyboardShouldPersistTaps="handled">
          <View style={{ gap: 8 }}>
            <Text style={[metin.sm, styles.label, { color: c.text2 }]}>Ne lazım?</Text>
            <View style={styles.chips}>
              {commonRequests.map((r) => {
                const on = title.trim() === r;
                return (
                  <Pressable
                    key={r}
                    accessibilityRole="button"
                    accessibilityState={{ selected: on }}
                    onPress={() => setTitle(r)}
                    hitSlop={{ top: 5, bottom: 5 }}
                    style={[styles.chip, { backgroundColor: on ? c.brandTint : c.surface3 }]}>
                    <Text style={[metin.sm, { fontFamily: 'Inter_600SemiBold', color: on ? c.brandText : c.text2 }]}>{r}</Text>
                  </Pressable>
                );
              })}
              <Pressable
                testID="request-other"
                accessibilityRole="button"
                onPress={() => {
                  if (commonRequests.includes(title.trim())) setTitle('');
                  titleRef.current?.focus();
                }}
                hitSlop={{ top: 5, bottom: 5 }}
                style={[styles.chip, { backgroundColor: c.surface3 }]}>
                <Text style={[metin.sm, { fontFamily: 'Inter_600SemiBold', color: c.text2 }]}>Diğer</Text>
              </Pressable>
            </View>
            <TextInput
              ref={titleRef}
              testID="request-title"
              value={title}
              onChangeText={setTitle}
              placeholder="Ya da yazın: örn. Kira kontratı"
              placeholderTextColor={c.text3}
              maxLength={200}
              accessibilityLabel="İstenen belge"
              style={input}
            />
          </View>
          <View style={{ gap: 8 }}>
            <Text style={[metin.sm, styles.label, { color: c.text2 }]}>Not (mükellef görür)</Text>
            <TextInput
              testID="request-note"
              value={note}
              onChangeText={setNote}
              multiline
              maxLength={1000}
              placeholder="Örn. Eylül ayı, tüm hesaplar"
              placeholderTextColor={c.text3}
              accessibilityLabel="Not, mükellef görür"
              style={[input, { minHeight: 80, paddingTop: 12, textAlignVertical: 'top' }]}
            />
          </View>
          <View style={{ gap: 8 }}>
            <Text style={[metin.sm, styles.label, { color: c.text2 }]}>Son gün</Text>
            <View style={{ flexDirection: 'row', gap: 8 }} accessibilityRole="radiogroup">
              {[...dueChoices, 'tarih' as const].map((d) => {
                const on = due === d;
                return (
                  <Pressable
                    key={d}
                    testID={`due-${d}`}
                    accessibilityRole="radio"
                    accessibilityState={{ selected: on }}
                    onPress={() => setDue(d)}
                    style={[styles.seg, on ? { backgroundColor: c.brand, borderColor: c.brand } : { backgroundColor: c.surface, borderColor: c.lineStrong }]}>
                    <Text style={[metin.sm, { fontFamily: 'Inter_600SemiBold', color: on ? c.onBrand : c.text }]}>{d === 'tarih' ? 'Tarih' : `${d} gün`}</Text>
                  </Pressable>
                );
              })}
            </View>
            {due === 'tarih' ? (
              <TextInput
                testID="request-date"
                value={dateText}
                onChangeText={setDateText}
                placeholder="GG.AA.YYYY"
                placeholderTextColor={c.text3}
                keyboardType="numbers-and-punctuation"
                accessibilityLabel="Son gün, gün nokta ay nokta yıl"
                style={input}
              />
            ) : null}
            {days !== null ? <Text style={[metin.xs, { color: c.text3 }]}>{dayLabel(dueDateFromDays(days))} gün sonuna kadar.</Text> : null}
          </View>
          <View style={{ gap: 8 }}>
            <Text style={[metin.sm, styles.label, { color: c.text2 }]}>Nasıl gönderilsin?</Text>
            <View style={styles.chips}>
              {channelOrder.map((ch) => {
                const on = channels.includes(ch);
                const locked = ch === 'push';
                return (
                  <Pressable
                    key={ch}
                    testID={`channel-${ch}`}
                    accessibilityRole="checkbox"
                    accessibilityState={{ checked: on, disabled: locked }}
                    disabled={locked}
                    onPress={() => setChannels((l) => toggleChannel(l, ch))}
                    hitSlop={{ top: 5, bottom: 5 }}
                    style={[styles.chip, { backgroundColor: on ? c.brandTint : c.surface3 }]}>
                    <Text style={[metin.sm, { fontFamily: 'Inter_600SemiBold', color: on ? c.brandText : c.text2 }]}>
                      {on ? '✓ ' : ''}
                      {channelLabel[ch]}
                    </Text>
                  </Pressable>
                );
              })}
            </View>
          </View>
          <View style={{ gap: 8 }}>
            <Text style={[metin.sm, styles.label, { color: c.text2 }]}>Ayrıca hatırlatılsın mı?</Text>
            <View style={{ flexDirection: 'row', gap: 8 }} accessibilityRole="radiogroup">
              {reminds.map((d) => {
                const on = remindDays === d;
                return (
                  <Pressable
                    key={String(d)}
                    testID={`remind-${d ?? 'yok'}`}
                    accessibilityRole="radio"
                    accessibilityState={{ selected: on }}
                    onPress={() => setRemind(d)}
                    style={[styles.seg, on ? { backgroundColor: c.brand, borderColor: c.brand } : { backgroundColor: c.surface, borderColor: c.lineStrong }]}>
                    <Text style={[metin.sm, { fontFamily: 'Inter_600SemiBold', color: on ? c.onBrand : c.text }]}>{remindLabel(d)}</Text>
                  </Pressable>
                );
              })}
            </View>
            {remindDays !== null ? <Text style={[metin.xs, { color: c.text3 }]}>Son günden {remindDays} gün önce seçilen kanallardan bir kez hatırlatılır.</Text> : null}
          </View>
          <Kart style={{ padding: 14, flexDirection: 'row', gap: 10, alignItems: 'flex-start', backgroundColor: c.surface2 }}>
            <Ikon ad="bildirim" boyut={16} renk={c.text3} />
            <Text style={[metin.xs, { color: c.text2, flex: 1 }]}>
              Mükellefe uygulama bildirimi her zaman gider; yükleyene kadar aralıklarla hatırlatılır. SMS ve e-posta seçerseniz ayrıca o yoldan da gönderilir (ofis ayarında kapalıysa yalnızca bildirim gider).
            </Text>
          </Kart>
          {touched && invalid ? <Notice>{invalid}</Notice> : null}
          {create.error ? <Notice>{errorMessage(create.error)}</Notice> : null}
          <View style={{ flex: 1 }} />
          <Pressable
            testID="request-submit"
            accessibilityRole="button"
            accessibilityState={{ busy: create.isPending, disabled: create.isPending }}
            disabled={create.isPending}
            onPress={() => void submit()}
            style={({ pressed }) => [styles.submit, { backgroundColor: c.brand, opacity: create.isPending ? 0.6 : pressed ? 0.85 : 1 }]}>
            <Text style={[metin.govde, { fontFamily: 'Inter_600SemiBold', color: c.onBrand }]}>{create.isPending ? 'Gönderiliyor…' : 'İsteği gönder'}</Text>
          </Pressable>
        </ScrollView>
      </KeyboardAvoidingView>
    </SafeAreaView>
  );
}

const styles = StyleSheet.create({
  hd: { flexDirection: 'row', alignItems: 'center', gap: 10, paddingHorizontal: 12, paddingVertical: 8 },
  icon: { width: DOKUNMA, height: DOKUNMA, alignItems: 'center', justifyContent: 'center', borderRadius: 10 },
  body: { padding: 16, gap: 18, flexGrow: 1, maxWidth: 640, width: '100%', alignSelf: 'center' },
  label: { fontFamily: 'Inter_600SemiBold' },
  chips: { flexDirection: 'row', flexWrap: 'wrap', gap: 8 },
  chip: { minHeight: 34, borderRadius: 999, paddingHorizontal: 12, justifyContent: 'center' },
  input: { minHeight: 46, borderWidth: 1, borderRadius: 10, paddingHorizontal: 14 },
  seg: { flex: 1, minHeight: DOKUNMA, borderRadius: 10, borderWidth: 1, alignItems: 'center', justifyContent: 'center' },
  submit: { minHeight: 52, borderRadius: 12, alignItems: 'center', justifyContent: 'center' },
});

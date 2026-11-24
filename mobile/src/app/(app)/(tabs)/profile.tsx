import { GorunumAyari, Kart } from '@hakan/mobile-core/ui/parcalar';
import { router } from 'expo-router';
import { useState } from 'react';
import { Linking, View } from 'react-native';

import { Button, Card, Field, Notice, Screen, Text } from '@/components/ui';
import { api } from '@/core/api/client';
import { errorMessage } from '@/core/api/envelope';
import { useAuth } from '@/core/auth/auth-provider';
import { env } from '@/core/config/env';
import { versionLabel } from '@/core/config/version';
import { useSupportChannel } from '@/features/support/use-support';

/**
 * Profil ve hesap (#202): gizlilik ve destek bağlantıları (yapılandırmadan),
 * çıkış ve uygulama içinden hesap silme (App Store 5.1.1(v)).
 *
 * Silme: /v1/auth/account/deactivate/request e-postaya kod gönderir, kod
 * /v1/auth/account/deactivate ile onaylanır. Hesap kapanır, oturumlar düşer;
 * muhasebe kayıtları müşavirde kalır (yasal saklama).
 */
export default function ProfileScreen() {
  const { me, signOut, taxpayers, taxpayer, switchTaxpayer } = useAuth();
  const [step, setStep] = useState<'idle' | 'code'>('idle');
  const [code, setCode] = useState('');
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);
  // Destek (#387): ClickFix widget'ı tanımlıysa uygulama içi ekran, yoksa
  // yapılandırmadaki destek bağlantısı.
  const support = useSupportChannel();
  const hasSupport = !!support.widgetId || !!env.supportUrl;

  async function run(fn: () => Promise<void>) {
    if (busy) return;
    setBusy(true);
    setError(null);
    try {
      await fn();
    } catch (e) {
      setError(errorMessage(e));
    } finally {
      setBusy(false);
    }
  }

  const requestCode = () =>
    run(async () => {
      await api.post('/v1/auth/account/deactivate/request', {});
      setStep('code');
    });

  const confirmDelete = () =>
    run(async () => {
      await api.post('/v1/auth/account/deactivate', { code: code.trim() });
      await signOut();
    });

  const open = (url: string) => {
    Linking.openURL(url).catch(() => setError('Bağlantı açılamadı.'));
  };

  return (
    <Screen style={{ maxWidth: 640, width: '100%', alignSelf: 'center' }}>
      <Text variant="title" accessibilityRole="header">
        Profil
      </Text>
      <Card>
        <Text variant="heading">
          {me?.user.name} {me?.user.surname}
        </Text>
        <Text tone="muted">{me?.user.email}</Text>
      </Card>

      <Card>
        {/* Mesajlarım (#780): ofisten gelen SMS ve e-postalar, her üyeye açık. */}
        <Button testID="open-messages" title="Mesajlarım" variant="ghost" onPress={() => router.push('/mesajlar')} />
      </Card>

      {env.privacyUrl || env.helpUrl || hasSupport || taxpayer?.role === 'owner' ? (
        <Card>
          {taxpayer?.role === 'owner' ? <Button testID="open-documents" title="Belgelerim" variant="ghost" onPress={() => router.push('/belgelerim')} /> : null}
          {taxpayer?.role === 'owner' ? <Button testID="open-payroll" title="Bordro belgeleri" variant="ghost" onPress={() => router.push('/payroll')} /> : null}
          {/* Yardım (#522): mükellef kılavuzu, web yardım merkezinde. */}
          {env.helpUrl ? <Button testID="open-help" title="Yardım" variant="ghost" onPress={() => open(`${env.helpUrl}/mukellef`)} /> : null}
          {env.privacyUrl ? <Button title="Gizlilik ve KVKK aydınlatma metni" variant="ghost" onPress={() => open(env.privacyUrl)} /> : null}
          {support.widgetId ? (
            <Button title="Destek" variant="ghost" onPress={() => router.push('/destek')} />
          ) : env.supportUrl ? (
            <Button title="Destek" variant="ghost" onPress={() => open(env.supportUrl)} />
          ) : null}
        </Card>
      ) : null}

      <Kart style={{ padding: 16 }}>
        <GorunumAyari />
      </Kart>

      {taxpayers.length > 1 ? <Button title="Şirket değiştir" variant="secondary" onPress={switchTaxpayer} /> : null}
      <Button title="Çıkış yap" variant="secondary" onPress={signOut} />

      <Card>
        <Text variant="heading">Hesabı sil</Text>
        <Text tone="muted">
          Hesabınız kapatılır ve bütün cihazlarda oturumunuz sonlanır. Müşavirinize gönderdiğiniz belgeler ve muhasebe kayıtları yasal
          saklama süresi boyunca müşavirinizde kalır.
        </Text>
        {error ? <Notice>{error}</Notice> : null}
        {step === 'idle' ? (
          <Button title="Hesabımı sil" variant="secondary" onPress={requestCode} loading={busy} />
        ) : (
          <View style={{ gap: 12 }}>
            <Text>E-posta adresinize gönderilen 6 haneli kodu girin.</Text>
            <Field label="Onay kodu" value={code} onChangeText={setCode} keyboardType="number-pad" textContentType="oneTimeCode" maxLength={32} />
            <Button title="Hesabı kalıcı olarak kapat" onPress={confirmDelete} loading={busy} disabled={code.trim().length < 4} />
            <Button title="Vazgeç" variant="ghost" onPress={() => setStep('idle')} disabled={busy} />
          </View>
        )}
      </Card>
      <Text variant="caption" tone="muted" style={{ textAlign: 'center' }} testID="app-version">
        {versionLabel()}
      </Text>
    </Screen>
  );
}

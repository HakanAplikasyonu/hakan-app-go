import { router } from 'expo-router';
import { useState } from 'react';
import { KeyboardAvoidingView, Platform, View } from 'react-native';

import { Button, Field, Notice, Screen, Text } from '@/components/ui';
import { request } from '@/core/api/client';
import { errorMessage } from '@/core/api/envelope';

/**
 * Parolamı unuttum (#202): /v1/auth/password/forgot sends a code by e-mail
 * (the answer never says whether the address exists), then
 * /v1/auth/password/reset sets the new password with that code.
 */
export default function ForgotPasswordScreen() {
  const [email, setEmail] = useState('');
  const [code, setCode] = useState('');
  const [password, setPassword] = useState('');
  const [step, setStep] = useState<'email' | 'code' | 'done'>('email');
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);

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

  const sendCode = () =>
    run(async () => {
      await request('POST', '/v1/auth/password/forgot', { body: { email: email.trim() }, anonymous: true });
      setStep('code');
    });

  const reset = () =>
    run(async () => {
      await request('POST', '/v1/auth/password/reset', {
        body: { email: email.trim(), code: code.trim(), password },
        anonymous: true,
      });
      setStep('done');
    });

  return (
    <KeyboardAvoidingView style={{ flex: 1 }} behavior={Platform.OS === 'ios' ? 'padding' : undefined}>
      <Screen style={{ justifyContent: 'center', maxWidth: 480, width: '100%', alignSelf: 'center' }}>
        <View style={{ gap: 6 }}>
          <Text variant="title">Parolamı unuttum</Text>
          <Text tone="muted">
            {step === 'email'
              ? 'Hesabınızın e-posta adresini girin; size bir doğrulama kodu gönderelim.'
              : step === 'code'
                ? `${email.trim()} adresine bir kod gönderdik (hesap varsa). Kodu ve yeni parolanızı girin.`
                : 'Parolanız değişti. Yeni parolanızla giriş yapabilirsiniz.'}
          </Text>
        </View>
        {error ? <Notice>{error}</Notice> : null}

        {step === 'email' ? (
          <>
            <Field
              label="E-posta"
              value={email}
              onChangeText={setEmail}
              autoCapitalize="none"
              autoComplete="email"
              keyboardType="email-address"
              textContentType="username"
              placeholder="ornek@sirket.com"
            />
            <Button title="Kod gönder" onPress={sendCode} loading={busy} disabled={email.trim().length < 4} />
          </>
        ) : null}

        {step === 'code' ? (
          <>
            <Field label="Doğrulama kodu" value={code} onChangeText={setCode} keyboardType="number-pad" textContentType="oneTimeCode" />
            <Field
              label="Yeni parola"
              value={password}
              onChangeText={setPassword}
              secureTextEntry
              textContentType="newPassword"
              autoComplete="new-password"
            />
            <Button title="Parolayı değiştir" onPress={reset} loading={busy} disabled={code.trim().length < 4 || password.length < 8} />
            <Button title="Kodu yeniden gönder" variant="ghost" onPress={sendCode} disabled={busy} />
          </>
        ) : null}

        <Button title={step === 'done' ? 'Girişe dön' : 'Vazgeç'} variant="secondary" onPress={() => router.back()} />
      </Screen>
    </KeyboardAvoidingView>
  );
}

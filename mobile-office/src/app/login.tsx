// Kaynak: mobile/src/app/login.tsx (#236).
import { useRef, useState } from 'react';
import { KeyboardAvoidingView, Platform, View } from 'react-native';

import { BrandMark } from '@/components/brand-mark';
import { Button, Field, Notice, Screen, Text, type InputRef } from '@/components/ui';
import { errorMessage, isApiError } from '@/core/api/envelope';
import { useAuth } from '@/core/auth/auth-provider';
import { brand } from '@/core/brand';
import { KodGirisi } from '@hakan/mobile-core/hakan';

export default function LoginScreen() {
  const { signIn } = useAuth();
  const [email, setEmail] = useState('');
  const [password, setPassword] = useState('');
  const [code, setCode] = useState('');
  const [needsCode, setNeedsCode] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);
  const passwordRef = useRef<InputRef>(null);

  const canSubmit = email.trim().length > 3 && password.length > 0 && (!needsCode || code.trim().length >= 6);

  async function submit(kod?: string) {
    const totp = (kod ?? code).trim();
    const ok = email.trim().length > 3 && password.length > 0 && (!needsCode || totp.length >= 6);
    if (!ok || busy) return;
    setBusy(true);
    setError(null);
    try {
      await signIn({ email: email.trim(), password, totp_code: needsCode ? totp : undefined });
    } catch (e) {
      if (isApiError(e) && e.code === 'MFA_REQUIRED') setNeedsCode(true);
      setError(errorMessage(e));
    } finally {
      setBusy(false);
    }
  }

  return (
    <KeyboardAvoidingView style={{ flex: 1 }} behavior={Platform.OS === 'ios' ? 'padding' : undefined}>
      <Screen style={{ justifyContent: 'center', maxWidth: 480, width: '100%', alignSelf: 'center' }}>
        <View style={{ gap: 12, marginBottom: 8 }}>
          <BrandMark size={64} />
          <Text variant="title">{brand.name}</Text>
          <Text tone="muted">{brand.tagline}</Text>
        </View>

        {error ? <Notice tone={needsCode && !code ? 'info' : 'danger'}>{error}</Notice> : null}

        <Field
          testID="login-email"
          label="E-posta"
          value={email}
          onChangeText={setEmail}
          autoCapitalize="none"
          autoComplete="email"
          keyboardType="email-address"
          textContentType="username"
          returnKeyType="next"
          onSubmitEditing={() => passwordRef.current?.focus()}
          placeholder="ad@ofis.com"
        />
        <Field
          testID="login-password"
          inputRef={passwordRef}
          label="Parola"
          value={password}
          onChangeText={setPassword}
          secureTextEntry
          autoComplete="password"
          textContentType="password"
          returnKeyType={needsCode ? 'next' : 'go'}
          onSubmitEditing={() => void submit()}
        />
        {needsCode ? (
          // #332 (Giris.dc.html): 6 haneli doğrulama kodu; son hane girilince
          // kendiliğinden gönderilir.
          <View style={{ gap: 10 }}>
            <Text variant="heading">Doğrulama kodu</Text>
            <Text tone="muted">Authenticator uygulamandaki 6 haneli kod.</Text>
            <KodGirisi value={code} onChangeText={setCode} onTamam={(kod) => void submit(kod)} testID="login-code" />
          </View>
        ) : null}

        <Button title={needsCode ? 'Doğrula' : 'Giriş yap'} onPress={() => void submit()} loading={busy} disabled={!canSubmit} />
        {needsCode ? (
          <Button
            title="Başka bir yolla gir"
            variant="ghost"
            onPress={() => {
              setNeedsCode(false);
              setCode('');
              setError(null);
            }}
          />
        ) : null}
        <Text variant="caption" tone="muted" style={{ textAlign: 'center' }}>
          Bu uygulama mali müşavirlik ofisi çalışanları içindir. Hesabınız yoksa ofis yöneticinizden davet isteyin.
        </Text>
      </Screen>
    </KeyboardAvoidingView>
  );
}

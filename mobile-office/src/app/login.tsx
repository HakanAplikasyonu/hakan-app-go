// Kaynak: mobile/src/app/login.tsx (#236).
import { useRef, useState } from 'react';
import { KeyboardAvoidingView, Platform, View } from 'react-native';

import { BrandMark } from '@/components/brand-mark';
import { Button, Field, Notice, Screen, Text, type InputRef } from '@/components/ui';
import { errorMessage, isApiError } from '@/core/api/envelope';
import { useAuth } from '@/core/auth/auth-provider';
import { brand } from '@/core/brand';

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

  async function submit() {
    if (!canSubmit || busy) return;
    setBusy(true);
    setError(null);
    try {
      await signIn({ email: email.trim(), password, totp_code: needsCode ? code.trim() : undefined });
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
          inputRef={passwordRef}
          label="Parola"
          value={password}
          onChangeText={setPassword}
          secureTextEntry
          autoComplete="password"
          textContentType="password"
          returnKeyType={needsCode ? 'next' : 'go'}
          onSubmitEditing={submit}
        />
        {needsCode ? (
          <Field
            label="Doğrulama kodu"
            value={code}
            onChangeText={setCode}
            keyboardType="number-pad"
            autoComplete="one-time-code"
            textContentType="oneTimeCode"
            maxLength={32}
            returnKeyType="go"
            onSubmitEditing={submit}
            autoFocus
          />
        ) : null}

        <Button title="Giriş yap" onPress={submit} loading={busy} disabled={!canSubmit} />
        <Text variant="caption" tone="muted" style={{ textAlign: 'center' }}>
          Bu uygulama mali müşavirlik ofisi çalışanları içindir. Hesabınız yoksa ofis yöneticinizden davet isteyin.
        </Text>
      </Screen>
    </KeyboardAvoidingView>
  );
}

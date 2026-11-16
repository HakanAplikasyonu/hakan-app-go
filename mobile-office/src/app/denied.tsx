import { useState } from 'react';
import { View } from 'react-native';

import { BrandMark } from '@/components/brand-mark';
import { Button, Card, Notice, Screen, Text } from '@/components/ui';
import { errorMessage } from '@/core/api/envelope';
import { useAuth } from '@/core/auth/auth-provider';

/** Ofis üyeliği olmayan hesap: mükellef hesabıysa mükellef uygulamasına yönlendirilir. */
export default function DeniedScreen() {
  const { me, access, reload, signOut } = useAuth();
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const taxpayer = access === 'taxpayerOnly';

  async function refresh() {
    setBusy(true);
    setError(null);
    try {
      await reload();
    } catch (e) {
      setError(errorMessage(e));
    } finally {
      setBusy(false);
    }
  }

  return (
    <Screen style={{ maxWidth: 560, width: '100%', alignSelf: 'center', justifyContent: 'center' }}>
      <View style={{ gap: 8 }}>
        <BrandMark size={44} />
        <Text variant="title">{taxpayer ? 'Bu bir mükellef hesabı' : 'Ofis üyeliğiniz yok'}</Text>
        {me?.user.email ? <Text tone="muted">{me.user.email}</Text> : null}
      </View>
      {error ? <Notice>{error}</Notice> : null}
      <Card>
        {taxpayer ? (
          <>
            <Text>
              Hakan Ofis yalnızca mali müşavirlik ofisi çalışanları içindir. Fiş ve belgelerinizi göndermek için mağazadan
              &quot;Hakan&quot; mükellef uygulamasını kullanın.
            </Text>
            <Text tone="muted" variant="caption">
              Bir ofiste de çalışıyorsanız ofis yöneticinizden davet isteyin.
            </Text>
          </>
        ) : (
          <>
            <Text>Hesabınız henüz bir mali müşavirlik ofisine bağlı değil.</Text>
            <Text tone="muted" variant="caption">
              Ofis yöneticinizin gönderdiği daveti e-postanızdan kabul edin, ardından bu ekranı yenileyin.
            </Text>
            <Button title="Yenile" variant="secondary" onPress={refresh} loading={busy} />
          </>
        )}
      </Card>
      <Button title="Çıkış yap" variant="ghost" onPress={signOut} />
    </Screen>
  );
}

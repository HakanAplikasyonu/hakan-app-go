import { compactAttributes } from '@hakan/mobile-core/support/clickfix';
import { ClickFixDestek } from '@hakan/mobile-core/ui/destek';
import { router } from 'expo-router';
import { ActivityIndicator, Linking, View } from 'react-native';

import { Button, Notice, Screen, Text } from '@/components/ui';
import { useAuth } from '@/core/auth/auth-provider';
import { env } from '@/core/config/env';
import { useColors } from '@/core/theme';
import { useSupportChannel } from '@/features/support/use-support';

/**
 * Destek (#387): ClickFix widget'ı WebView'de. Talep mükellefin kendi
 * ofisine gider; ad, e-posta, şirket ve ofis bilgisi widget'a geçer.
 */
export default function DestekScreen() {
  const c = useColors();
  const { me, taxpayer } = useAuth();
  const { loading, widgetId, officeName } = useSupportChannel();
  const close = () => (router.canGoBack() ? router.back() : router.replace('/'));

  if (loading) {
    return (
      <View style={{ flex: 1, alignItems: 'center', justifyContent: 'center', backgroundColor: c.background }}>
        <ActivityIndicator color={c.brand} />
      </View>
    );
  }
  if (!widgetId || !me || !taxpayer) {
    return (
      <Screen>
        <Text variant="heading">Destek</Text>
        <Notice tone="info">Ofisiniz henüz uygulama içi destek kanalı tanımlamadı.</Notice>
        {env.supportUrl ? <Button title="Destek bağlantısını aç" onPress={() => Linking.openURL(env.supportUrl).catch(() => {})} /> : null}
      </Screen>
    );
  }
  return (
    <View style={{ flex: 1, backgroundColor: c.background }}>
      <ClickFixDestek
        widgetId={widgetId}
        embedUrl={env.clickfixEmbedUrl}
        userKey={`${me.user.uuid}:${taxpayer.taxpayer.uuid}`}
        visitor={{
          name: [me.user.name, me.user.surname].filter(Boolean).join(' ') || undefined,
          email: me.user.email || undefined,
          attributes: compactAttributes({
            portal: 'mukellef-mobil',
            mukellef: taxpayer.taxpayer.title,
            mukellef_id: taxpayer.taxpayer.uuid,
            ofis: officeName,
            rol: taxpayer.role,
          }),
        }}
        onClose={close}
      />
    </View>
  );
}

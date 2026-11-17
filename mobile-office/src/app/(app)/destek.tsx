import { compactAttributes } from '@hakan/mobile-core/support/clickfix';
import { ClickFixDestek } from '@hakan/mobile-core/ui/destek';
import { router } from 'expo-router';
import { ActivityIndicator, View } from 'react-native';

import { Notice, Screen, Text } from '@/components/ui';
import { useAuth } from '@/core/auth/auth-provider';
import { env } from '@/core/config/env';
import { useColors } from '@/core/theme';
import { providerWidgetId, useSupportIdentity } from '@/features/support/use-support';

/**
 * Destek (#387): ofisin talebi yazılım sağlayıcısına gider (ClickFix
 * widget'ı WebView'de). Ad, e-posta ve ofis bilgisi widget'a geçer.
 */
export default function DestekScreen() {
  const c = useColors();
  const { me, office } = useAuth();
  const { loading, identity } = useSupportIdentity();
  const close = () => (router.canGoBack() ? router.back() : router.replace('/'));

  if (!providerWidgetId || !me || !office) {
    return (
      <Screen>
        <Text variant="heading">Destek</Text>
        <Notice tone="info">Uygulama içi destek bu sürümde tanımlı değil.</Notice>
      </Screen>
    );
  }
  if (loading) {
    return (
      <View style={{ flex: 1, alignItems: 'center', justifyContent: 'center', backgroundColor: c.background }}>
        <ActivityIndicator color={c.brand} />
      </View>
    );
  }
  return (
    <View style={{ flex: 1, backgroundColor: c.background }}>
      <ClickFixDestek
        widgetId={providerWidgetId}
        embedUrl={env.clickfixEmbedUrl}
        userKey={`${me.user.uuid}:${office.organization.uuid}`}
        identity={identity}
        visitor={{
          name: [me.user.name, me.user.surname].filter(Boolean).join(' ') || undefined,
          email: me.user.email || undefined,
          attributes: compactAttributes({
            portal: 'ofis-mobil',
            ofis: office.organization.name,
            ofis_kodu: office.organization.slug,
            ofis_id: office.organization.uuid,
            rol: office.role,
          }),
        }}
        onClose={close}
      />
    </View>
  );
}

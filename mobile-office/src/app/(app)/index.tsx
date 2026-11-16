import { View } from 'react-native';

import { BrandMark } from '@/components/brand-mark';
import { Button, Card, Screen, Text } from '@/components/ui';
import { useAuth } from '@/core/auth/auth-provider';

/** Ana sayfa. Pano kartları #237'de gelir. */
export default function HomeScreen() {
  const { me, office, offices, switchOffice, signOut } = useAuth();
  return (
    <Screen>
      <View style={{ flexDirection: 'row', alignItems: 'center', gap: 12 }}>
        <BrandMark size={40} />
        <View style={{ flex: 1 }}>
          <Text variant="heading" numberOfLines={1}>
            {office?.organization.name}
          </Text>
          <Text variant="caption" tone="muted">
            {me?.user.name} {me?.user.surname} · {office?.role}
          </Text>
        </View>
      </View>
      <Card>
        <Text variant="heading">Hoş geldiniz</Text>
        <Text tone="muted">Pano, uyarılar ve inceleme kuyruğu bu ekranda yer alacak.</Text>
      </Card>
      <View style={{ flex: 1 }} />
      {offices.length > 1 ? <Button title="Ofis değiştir" variant="secondary" onPress={switchOffice} /> : null}
      <Button title="Çıkış yap" variant="ghost" onPress={signOut} />
    </Screen>
  );
}

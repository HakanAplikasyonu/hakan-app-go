import { KristalYukleyici } from '@hakan/mobile-core/hakan';
import { useState } from 'react';
import { Alert, View } from 'react-native';

import { Button, Card, Field, Notice, Screen, Text } from '@/components/ui';
import { errorMessage } from '@/core/api/envelope';
import { useVehicles, type Vehicle } from '@/features/vehicles/api';
import { formatPlate, normalizePlate, plateValid } from '@/features/vehicles/plaka';

/**
 * Araçlarım (Fiş yapay zekâ kontrolleri): şirketin araçları. Akaryakıt
 * fişindeki plaka burada yoksa fiş "Plaka mükellefin araçlarında yok"
 * uyarısı alır. Liste müşavirle ortaktır.
 */
export default function AraclarScreen() {
  const list = useVehicles();
  const [plate, setPlate] = useState('');
  const [description, setDescription] = useState('');
  const [error, setError] = useState<string | null>(null);

  const add = () => {
    if (!plateValid(plate)) {
      setError('Plakayı 34 ABC 123 biçiminde yazın.');
      return;
    }
    setError(null);
    list.add.mutate(
      { plate: normalizePlate(plate), description: description.trim() },
      {
        onSuccess: () => {
          setPlate('');
          setDescription('');
        },
        onError: (e) => setError(errorMessage(e)),
      },
    );
  };

  const remove = (v: Vehicle) =>
    Alert.alert('Aracı kaldır', `${formatPlate(v.plate)} araç listenizden kaldırılsın mı?`, [
      { text: 'Vazgeç', style: 'cancel' },
      {
        text: 'Kaldır',
        style: 'destructive',
        onPress: () => list.remove.mutate(v.uuid, { onError: (e) => setError(errorMessage(e)) }),
      },
    ]);

  return (
    <Screen style={{ maxWidth: 640, width: '100%', alignSelf: 'center' }}>
      <Text tone="muted">
        Şirketinizin araçları. Akaryakıt fişindeki plaka bu listede yoksa fiş uyarıyla müşavirinize gider. Listeyi müşaviriniz de görür.
      </Text>

      <Card>
        <Text variant="heading">Araç ekle</Text>
        <Field
          testID="vehicle-plate"
          label="Plaka"
          value={plate}
          onChangeText={setPlate}
          placeholder="34 ABC 123"
          autoCapitalize="characters"
          autoCorrect={false}
        />
        <Field testID="vehicle-description" label="Açıklama (isteğe bağlı)" value={description} onChangeText={setDescription} placeholder="Binek, kamyonet…" />
        {error ? <Notice>{error}</Notice> : null}
        <Button testID="vehicle-add" title="Ekle" onPress={add} loading={list.add.isPending} disabled={!plate.trim()} />
      </Card>

      {list.isLoading ? (
        <View style={{ alignItems: 'center', paddingVertical: 24 }}>
          <KristalYukleyici etiket="Araçlar yükleniyor" />
        </View>
      ) : list.error ? (
        <Notice>{errorMessage(list.error)}</Notice>
      ) : list.items.length === 0 ? (
        <Text tone="muted" testID="vehicles-empty">
          Henüz araç eklenmedi.
        </Text>
      ) : (
        <Card>
          {list.items.map((v) => (
            <View key={v.uuid} testID="vehicle-row" style={{ flexDirection: 'row', alignItems: 'center', gap: 12 }}>
              <View style={{ flex: 1 }}>
                <Text>{formatPlate(v.plate)}</Text>
                {v.description ? (
                  <Text variant="caption" tone="muted">
                    {v.description}
                  </Text>
                ) : null}
              </View>
              <Button title="Kaldır" variant="ghost" onPress={() => remove(v)} disabled={list.remove.isPending} />
            </View>
          ))}
        </Card>
      )}
    </Screen>
  );
}

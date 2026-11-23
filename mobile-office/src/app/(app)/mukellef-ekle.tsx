import * as ImagePicker from 'expo-image-picker';
import { useRouter } from 'expo-router';
import { useState } from 'react';
import { View } from 'react-native';

import { Button, Card, Field, Notice, Screen, Text } from '@/components/ui';
import { errorMessage } from '@/core/api/envelope';
import { useAuth } from '@/core/auth/auth-provider';
import { SOON_MESSAGE } from '@/features/doctasks/channels';
import { useCreateTaxpayerRequest, useReadCertificate, type PickedImage } from '@/features/taxpayers/api';
import {
  applyReading,
  emptyForm,
  taxIdKind,
  validateNewTaxpayer,
  type CertificateField,
  type NewTaxpayerForm,
} from '@/features/taxpayers/certificate';

const CHECK = 'Kontrol et: levhadan okunamadı.';

/**
 * Mükellef ekle (#780): vergi levhasının fotoğrafı çekilir ya da seçilir,
 * yapay zekâ VKN/TCKN, unvan, vergi dairesi, adres ve faaliyet kodunu okur;
 * müşavir kontrol edip kaydeder. Kayıt mükellefe onay e-postası gönderir.
 */
export default function AddTaxpayerScreen() {
  const router = useRouter();
  const { can } = useAuth();
  const read = useReadCertificate();
  const create = useCreateTaxpayerRequest();
  const [form, setForm] = useState<NewTaxpayerForm>(emptyForm);
  const [unread, setUnread] = useState<CertificateField[]>([]);
  const [readDone, setReadDone] = useState(false);
  const [soon, setSoon] = useState(false);
  const [denied, setDenied] = useState(false);
  const [touched, setTouched] = useState(false);
  const errors = validateNewTaxpayer(form);
  const hasErrors = Object.keys(errors).length > 0;

  const set = (k: keyof NewTaxpayerForm) => (v: string) => setForm((f) => ({ ...f, [k]: v }));
  const check = (k: CertificateField) => (readDone && unread.includes(k) && !form[k].trim() ? CHECK : null);

  async function scan(file: PickedImage) {
    setSoon(false);
    try {
      const r = await read.mutateAsync(file);
      if (r === 'soon') {
        setSoon(true);
        return;
      }
      const next = applyReading(form, r);
      setForm(next.form);
      setUnread(next.unread);
      setReadDone(true);
    } catch {
      // read.error gösterilir
    }
  }

  function fromAsset(a: ImagePicker.ImagePickerAsset) {
    const mimeType = a.mimeType ?? 'image/jpeg';
    void scan({ uri: a.uri, name: a.fileName ?? `vergi-levhasi.${mimeType.split('/')[1] ?? 'jpg'}`, mimeType });
  }

  async function shoot() {
    const perm = await ImagePicker.requestCameraPermissionsAsync();
    if (!perm.granted) {
      setDenied(true);
      return;
    }
    setDenied(false);
    const res = await ImagePicker.launchCameraAsync({ mediaTypes: ['images'], quality: 0.8 });
    if (!res.canceled && res.assets[0]) fromAsset(res.assets[0]);
  }

  async function gallery() {
    const res = await ImagePicker.launchImageLibraryAsync({ mediaTypes: ['images'], quality: 0.8 });
    if (!res.canceled && res.assets[0]) fromAsset(res.assets[0]);
  }

  async function save() {
    setTouched(true);
    if (hasErrors) return;
    try {
      await create.mutateAsync(form);
    } catch {
      // create.error gösterilir
    }
  }

  if (!can('tenant.engagements.write')) {
    return (
      <Screen>
        <Notice>Mükellef eklemek için yetkiniz yok.</Notice>
      </Screen>
    );
  }

  if (create.data) {
    return (
      <Screen>
        <Card>
          <Text variant="heading">Onay isteği gönderildi</Text>
          <Text>
            {create.data.request.title} için {create.data.request.contact_email} adresine onay bağlantısı gitti. Mükellef onaylayınca listenizde görünür.
          </Text>
        </Card>
        <Button title="Tamam" onPress={() => router.back()} />
      </Screen>
    );
  }

  const kind = taxIdKind(form.tax_id);
  return (
    <Screen>
      <Card>
        <Text variant="heading">Vergi levhasından doldur</Text>
        <Text tone="muted">Levhanın fotoğrafını çekin ya da galeriden seçin; bilgiler okunup aşağıya yazılır. Kaydetmeden önce kontrol edin.</Text>
        <View style={{ flexDirection: 'row', gap: 10 }}>
          <View style={{ flex: 1 }}>
            <Button title="Fotoğraf çek" testID="certificate-camera" onPress={() => void shoot()} loading={read.isPending} />
          </View>
          <View style={{ flex: 1 }}>
            <Button title="Galeriden seç" variant="secondary" testID="certificate-gallery" onPress={() => void gallery()} disabled={read.isPending} />
          </View>
        </View>
        {read.isPending ? <Text tone="muted">Levha okunuyor…</Text> : null}
        {denied ? <Notice>Kamera izni verilmedi. Ayarlardan izin verin ya da galeriden seçin.</Notice> : null}
        {soon ? <Notice tone="info">{`Levhadan okuma: ${SOON_MESSAGE} Bilgileri elle yazabilirsiniz.`}</Notice> : null}
        {read.error ? <Notice>{errorMessage(read.error)}</Notice> : null}
        {readDone ? (
          <Notice tone="info">
            {unread.length ? 'Okunan bilgileri kontrol edin; okunamayan alanlar boş bırakıldı.' : 'Levha okundu; bilgileri kontrol edip kaydedin.'}
          </Notice>
        ) : null}
      </Card>

      <Field
        label={kind === 'individual' ? 'TCKN' : kind === 'legal' ? 'VKN' : 'VKN / TCKN'}
        testID="taxpayer-tax-id"
        value={form.tax_id}
        onChangeText={set('tax_id')}
        keyboardType="number-pad"
        maxLength={14}
        error={(touched ? errors.tax_id : null) || check('tax_id')}
      />
      <Field
        label="Unvan / ad soyad"
        testID="taxpayer-title"
        value={form.title}
        onChangeText={set('title')}
        maxLength={255}
        error={(touched ? errors.title : null) || check('title')}
      />
      <Field
        label="Mükellefin e-postası (onay bağlantısı gider)"
        testID="taxpayer-email"
        value={form.contact_email}
        onChangeText={set('contact_email')}
        keyboardType="email-address"
        autoCapitalize="none"
        autoCorrect={false}
        error={touched ? errors.contact_email : null}
      />
      <Field label="Vergi dairesi" value={form.tax_office} onChangeText={set('tax_office')} error={check('tax_office')} />
      <Field label="Adres" value={form.address} onChangeText={set('address')} multiline style={{ minHeight: 72 }} error={check('address')} />
      <Field label="Faaliyet kodu (NACE)" value={form.activity_code} onChangeText={set('activity_code')} error={check('activity_code')} />
      <Text variant="caption" tone="muted">
        Vergi dairesi, adres ve faaliyet kodu bu adımda kaydedilmez; mükellef onaylayınca mükellef kartından girilir.
      </Text>

      {create.error ? <Notice>{errorMessage(create.error)}</Notice> : null}
      <Button title="Kaydet ve onay iste" testID="taxpayer-save" onPress={() => void save()} loading={create.isPending} />
    </Screen>
  );
}

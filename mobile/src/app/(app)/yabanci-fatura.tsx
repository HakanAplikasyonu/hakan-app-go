import { BosDurum, KristalYukleyici } from '@hakan/mobile-core/hakan';
import { Ikon } from '@hakan/mobile-core/ui/ikon';
import { Hap, Kart, metin, Satir } from '@hakan/mobile-core/ui/parcalar';
import { useTheme } from '@hakan/mobile-core/ui/theme';
import * as DocumentPicker from 'expo-document-picker';
import * as ImagePicker from 'expo-image-picker';
import { useState } from 'react';
import { RefreshControl, ScrollView, Text, View } from 'react-native';
import { SafeAreaView } from 'react-native-safe-area-context';

import { Button, Field, Notice } from '@/components/ui';
import { useForeignInvoices, useUploadForeignInvoice, type PickedFile } from '@/features/kdv/api';
import { alindiMetni, AZAMI_BAYT, DURUM, tarih, yuklemeHatasi } from '@/features/kdv/ozet';

/**
 * Yabancı hizmet faturası gönder (#780 KDV2): Google Ads, Meta, AWS, yurt
 * dışı yazılım lisansı gibi hizmetlerin faturası fotoğraf ya da PDF olarak
 * ofise gider; KDV2'yi müşavir beyan eder. Altta bu ay gönderilenler, salt
 * okunur. Ofis bölümü kapattıysa ekran bunu söyler, gönderim gösterilmez.
 */
export default function YabanciFaturaScreen() {
  const { colors: c } = useTheme();
  const list = useForeignInvoices();
  const send = useUploadForeignInvoice();
  const [file, setFile] = useState<PickedFile | null>(null);
  const [note, setNote] = useState('');
  const [message, setMessage] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [denied, setDenied] = useState(false);

  function choose(f: PickedFile, size?: number | null) {
    setMessage(null);
    if (size && size > AZAMI_BAYT) {
      setError("Dosya 20 MB'tan büyük. Daha küçük bir fotoğraf ya da PDF seçin.");
      return;
    }
    setError(null);
    setFile(f);
  }

  function fromImage(a: ImagePicker.ImagePickerAsset) {
    const mimeType = a.mimeType ?? 'image/jpeg';
    choose({ uri: a.uri, name: a.fileName ?? `fatura.${mimeType.split('/')[1] ?? 'jpg'}`, mimeType }, a.fileSize);
  }

  async function shoot() {
    const perm = await ImagePicker.requestCameraPermissionsAsync();
    if (!perm.granted) {
      setDenied(true);
      return;
    }
    const res = await ImagePicker.launchCameraAsync({ mediaTypes: ['images'], quality: 0.8 });
    if (!res.canceled && res.assets[0]) fromImage(res.assets[0]);
  }

  async function gallery() {
    const res = await ImagePicker.launchImageLibraryAsync({ mediaTypes: ['images'], quality: 0.8 });
    if (!res.canceled && res.assets[0]) fromImage(res.assets[0]);
  }

  async function pdf() {
    const res = await DocumentPicker.getDocumentAsync({ type: 'application/pdf', copyToCacheDirectory: true });
    const a = res.canceled ? undefined : res.assets[0];
    if (a) choose({ uri: a.uri, name: a.name || 'fatura.pdf', mimeType: a.mimeType ?? 'application/pdf' }, a.size);
  }

  function submit() {
    if (!file) return;
    setError(null);
    setMessage(null);
    send.mutate(
      { file, note },
      {
        onSuccess: (res) => {
          setFile(null);
          setNote('');
          setMessage(
            res.replayed
              ? 'Bu dosyayı daha önce göndermişsiniz; yeni kayıt açılmadı.'
              : 'Faturanız ofisinize gönderildi; ofis inceleyip işleyecek.',
          );
        },
        onError: (err) => setError(yuklemeHatasi(err)),
      },
    );
  }

  const off = !!list.data && !list.data.enabled;

  return (
    <SafeAreaView style={{ flex: 1, backgroundColor: c.bg }} edges={['left', 'right', 'bottom']}>
      <ScrollView
        testID="foreign-screen"
        contentContainerStyle={{ padding: 16, gap: 12, maxWidth: 720, width: '100%', alignSelf: 'center' }}
        keyboardShouldPersistTaps="handled"
        refreshControl={<RefreshControl refreshing={list.isRefetching} onRefresh={() => void list.refetch()} tintColor={c.brandText} />}>
        <Text style={[metin.sm, { color: c.text2 }]}>
          Google Ads, Meta, AWS, yurt dışı yazılım lisansı gibi yurt dışından aldığınız hizmetlerin faturalarını buradan gönderin. Bu
          faturalar için KDV&apos;yi (KDV2) mali müşaviriniz sizin adınıza beyan eder; fatura eksik kalırsa beyan eksik olur.
        </Text>

        {list.isLoading ? (
          <View style={{ alignItems: 'center', paddingVertical: 32 }}>
            <KristalYukleyici etiket="Yabancı faturalar yükleniyor" />
          </View>
        ) : off || list.isError ? (
          <BosDurum
            baslik={off ? 'Ofisiniz bu bölümü kapattı' : 'Yabancı faturalar şu an alınamadı'}
            aciklama={
              off
                ? 'Ofisiniz yabancı fatura gönderimini kapattı. Faturayı ofisinize başka yoldan iletin.'
                : 'Şirketinizin bağlı bir ofisi olmayabilir ya da bağlantı yok. Aşağı çekerek yeniden deneyin.'
            }
          />
        ) : (
          <>
            <Kart style={{ padding: 14, gap: 10 }}>
              <Text accessibilityRole="header" style={[metin.govde, { fontFamily: 'Inter_700Bold', color: c.text }]}>
                Fatura seç
              </Text>
              <View style={{ flexDirection: 'row', gap: 8 }}>
                <View style={{ flex: 1 }}>
                  <Button testID="foreign-camera" title="Fotoğraf çek" variant="secondary" onPress={() => void shoot()} />
                </View>
                <View style={{ flex: 1 }}>
                  <Button testID="foreign-gallery" title="Galeriden" variant="secondary" onPress={() => void gallery()} />
                </View>
                <View style={{ flex: 1 }}>
                  <Button testID="foreign-pdf" title="PDF seç" variant="secondary" onPress={() => void pdf()} />
                </View>
              </View>
              {denied ? <Notice tone="info">Kamera izni verilmedi. Telefon ayarlarından izin verebilir ya da galeriden seçebilirsiniz.</Notice> : null}
              {file ? (
                <Satir
                  testID="foreign-picked"
                  ilk
                  sol={<Ikon ad={file.mimeType === 'application/pdf' ? 'klasor' : 'fisCek'} renk={c.text2} />}
                  baslik={file.name}
                  alt="Gönderilmeye hazır"
                />
              ) : null}
              <Field
                testID="foreign-note"
                label="Not (isteğe bağlı)"
                placeholder="Örn. Ekim ayı Instagram reklam faturası"
                value={note}
                onChangeText={setNote}
                maxLength={1000}
                multiline
              />
              {error ? <Notice>{error}</Notice> : null}
              {message ? <Notice tone="info">{message}</Notice> : null}
              <Button testID="foreign-send" title="Faturayı gönder" disabled={!file} loading={send.isPending} onPress={submit} />
            </Kart>

            <Kart testID="foreign-list">
              <Text accessibilityRole="header" style={[metin.govde, { fontFamily: 'Inter_700Bold', color: c.text, padding: 14 }]}>
                {alindiMetni(list.count)}
              </Text>
              {list.items.map((f, i) => (
                <Satir
                  key={f.uuid}
                  testID="foreign-row"
                  ilk={i === 0}
                  baslik={f.vendor || 'Fatura (ofiste okunacak)'}
                  alt={[
                    f.invoice_date ? `Fatura tarihi ${tarih(f.invoice_date)}` : `Gönderildi ${tarih(f.created_at)}`,
                    f.source_kind === 'whatsapp' ? "WhatsApp'tan" : null,
                  ]
                    .filter(Boolean)
                    .join(' · ')}
                  sag={<Hap metin={DURUM[f.status]} ton={f.status === 'processed' ? 'ok' : 'notr'} />}
                />
              ))}
            </Kart>
          </>
        )}
      </ScrollView>
    </SafeAreaView>
  );
}

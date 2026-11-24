import { HataDurumu, KristalYukleyici } from '@hakan/mobile-core/hakan';
import { useTheme } from '@hakan/mobile-core/ui/theme';
import type { File } from 'expo-file-system';
import { Image } from 'expo-image';
import { Stack, useLocalSearchParams } from 'expo-router';
import { useEffect, useMemo, useRef, useState } from 'react';
import { Linking, Platform, Pressable, ScrollView, Share, StyleSheet, Text, useWindowDimensions, View } from 'react-native';
import { SafeAreaView } from 'react-native-safe-area-context';
import { WebView, type WebViewMessageEvent } from 'react-native-webview';

import { Button, Notice } from '@/components/ui';
import { useAuth } from '@/core/auth/auth-provider';
import { base64, belgeHatasi, belgeTuru, gecerliBelgeYolu, gecerliYerelDosya, paylasimAdi, type BelgeHatasi, type BelgeKaynagi, type BelgeTuru } from '@/features/documents/belge';
import { gorselSayfasi, pdfSayfasi, type SayfaIletisi } from '@/features/documents/html';
import { belgeyiIndir, onbellegiTemizle, onbellektenSil, paylasimKopyasi, yerelDosyayiOku, type IndirilenBelge } from '@/features/documents/indir';

type Durum =
  | { t: 'yukleniyor' }
  | { t: 'hata'; hata: BelgeHatasi }
  | { t: 'hazir'; file: File; contentType: string; tur: BelgeTuru; html: string | null };

type Parametreler = {
  title?: string;
  path?: string;
  localUri?: string;
  fileName?: string;
  contentType?: string;
  size?: string;
  id?: string;
  prefix?: string;
};

/**
 * Uygulama içi belge görüntüleyici (#780). Belge yetkili olarak önbelleğe
 * iner; PDF uygulamanın içinde (iOS: WKWebView, Android: pdf.js), görsel tam
 * ekran ve iki parmakla yakınlaştırılarak gösterilir. Üstteki "Paylaş /
 * Kaydet" sistemin paylaşım sayfasını (iOS) ya da dosyayı açabilen
 * uygulamaları (Android) açar. Ekran kapanınca indirilen kopya silinir.
 */
export default function BelgeEkrani() {
  const p = useLocalSearchParams<Parametreler>();
  const { colors: c } = useTheme();
  const { taxpayer } = useAuth();
  const tp = taxpayer?.taxpayer.uuid ?? '';
  const { width, height } = useWindowDimensions();
  const [durum, setDurum] = useState<Durum>({ t: 'yukleniyor' });
  const [deneme, setDeneme] = useState(0);
  const [onizlemeYok, setOnizlemeYok] = useState(false);
  const [paylasHatasi, setPaylasHatasi] = useState<string | null>(null);
  const kopyalar = useRef<File[]>([]);

  const kaynak = useMemo<BelgeKaynagi & { id: string; prefix: string }>(
    () => ({
      title: p.title || 'Belge',
      path: p.path || undefined,
      localUri: p.localUri || undefined,
      fileName: p.fileName || null,
      contentType: p.contentType || null,
      size: p.size ? Number(p.size) : null,
      id: p.id || 'belge',
      prefix: p.prefix || 'belge',
    }),
    [p.title, p.path, p.localUri, p.fileName, p.contentType, p.size, p.id, p.prefix],
  );

  useEffect(() => {
    let iptal = false;
    let indirilen: File | null = null;
    setDurum({ t: 'yukleniyor' });
    setOnizlemeYok(false);
    (async () => {
      try {
        let b: IndirilenBelge;
        if (kaynak.localUri && gecerliYerelDosya(kaynak.localUri)) {
          b = await yerelDosyayiOku(kaynak.localUri, kaynak.contentType, kaynak.fileName);
        } else if (kaynak.path && gecerliBelgeYolu(kaynak.path)) {
          b = await belgeyiIndir({ ...kaynak, path: kaynak.path }, kaynak.id, kaynak.prefix, tp);
          indirilen = b.file;
        } else {
          setDurum({ t: 'hata', hata: { code: 'INVALID_DOCUMENT_PATH', message: 'Belge bağlantısı geçersiz.' } });
          return;
        }
        if (iptal) {
          onbellektenSil(indirilen);
          return;
        }
        onbellegiTemizle(b.file.name);
        const tur = belgeTuru(b.contentType, kaynak.fileName);
        // Android WebView yerel dosyayı okuyamaz ve PDF çizemez: veri sayfaya gömülür.
        const html = Platform.OS === 'android' && tur !== 'other' ? (tur === 'pdf' ? pdfSayfasi(base64(b.bytes)) : gorselSayfasi(base64(b.bytes), b.contentType)) : null;
        setDurum({ t: 'hazir', file: b.file, contentType: b.contentType, tur, html });
      } catch (e) {
        if (!iptal) setDurum({ t: 'hata', hata: belgeHatasi(e) });
      }
    })();
    return () => {
      iptal = true;
      onbellektenSil(indirilen);
    };
  }, [kaynak, tp, deneme]);

  // Paylaşım için açılan adlı kopyalar ekran kapanınca silinir.
  useEffect(
    () => () => {
      for (const f of kopyalar.current) onbellektenSil(f);
    },
    [],
  );

  async function paylas() {
    if (durum.t !== 'hazir') return;
    setPaylasHatasi(null);
    try {
      const f = paylasimKopyasi(durum.file, paylasimAdi(kaynak.title, kaynak.fileName, durum.contentType));
      if (f !== durum.file) kopyalar.current.push(f);
      if (Platform.OS === 'ios') {
        await Share.share({ url: f.uri, title: kaynak.title });
      } else {
        await Linking.openURL(f.contentUri ?? f.uri);
      }
    } catch {
      setPaylasHatasi('Paylaşım açılamadı. Telefonunuzda bu dosyayı açabilen bir uygulama olmayabilir. (Hata kodu: SHARE_FAILED)');
    }
  }

  function ileti(e: WebViewMessageEvent) {
    const m = e.nativeEvent.data as SayfaIletisi;
    if (m === 'hata' || m === 'pdfjs-yok') setOnizlemeYok(true);
  }

  const hazir = durum.t === 'hazir';

  return (
    <SafeAreaView style={{ flex: 1, backgroundColor: hazir && durum.tur === 'image' ? '#000000' : c.bg }} edges={['left', 'right', 'bottom']}>
      <Stack.Screen
        options={{
          title: kaynak.title,
          headerRight: () =>
            hazir ? (
              <Pressable testID="viewer-share" accessibilityRole="button" accessibilityLabel="Belgeyi paylaş ya da kaydet" hitSlop={12} onPress={() => void paylas()}>
                <Text style={{ color: c.brandText, fontFamily: 'Inter_600SemiBold', fontSize: 15 }}>Paylaş / Kaydet</Text>
              </Pressable>
            ) : null,
        }}
      />
      {paylasHatasi ? (
        <View style={{ padding: 16 }}>
          <Notice>{paylasHatasi}</Notice>
        </View>
      ) : null}

      {durum.t === 'yukleniyor' ? (
        <View style={styles.orta}>
          <KristalYukleyici etiket="Belge indiriliyor" gecikme={0} />
        </View>
      ) : null}

      {durum.t === 'hata' ? (
        <View testID="viewer-error" style={styles.orta}>
          <HataDurumu
            baslik="Belge açılamadı"
            aciklama={`${durum.hata.message} (Hata kodu: ${durum.hata.code})`}
            eylem={<Button testID="viewer-retry" title="Tekrar dene" onPress={() => setDeneme((n) => n + 1)} />}
          />
        </View>
      ) : null}

      {hazir && onizlemeYok ? (
        <View style={{ padding: 16, gap: 12 }}>
          <Notice tone="info">Önizleme bu telefonda gösterilemedi (internet bağlantısı gerekebilir). Belgeyi başka bir uygulamada açabilirsiniz.</Notice>
          <Button testID="viewer-other-app" title="Başka uygulamada aç" variant="secondary" onPress={() => void paylas()} />
        </View>
      ) : null}

      {hazir && durum.tur === 'other' ? (
        <View style={[styles.orta, { gap: 12, padding: 24 }]}>
          <Text style={{ color: c.text, textAlign: 'center', fontSize: 15 }}>Bu dosya türü uygulamada gösterilemiyor.</Text>
          <Button testID="viewer-other-app" title="Paylaş / Kaydet" onPress={() => void paylas()} />
        </View>
      ) : null}

      {hazir && durum.html && !onizlemeYok ? (
        <WebView
          testID={durum.tur === 'pdf' ? 'viewer-pdf' : 'viewer-image'}
          style={{ flex: 1, backgroundColor: durum.tur === 'pdf' ? '#525659' : '#000000' }}
          originWhitelist={['about:blank', 'data:*', 'https://cdn.jsdelivr.net/*']}
          source={{ html: durum.html }}
          onMessage={ileti}
          onError={() => setOnizlemeYok(true)}
          javaScriptEnabled
          setSupportMultipleWindows={false}
          allowFileAccess={false}
        />
      ) : null}

      {hazir && !durum.html && durum.tur === 'pdf' && !onizlemeYok ? (
        <WebView
          testID="viewer-pdf"
          style={{ flex: 1 }}
          originWhitelist={['file://*']}
          source={{ uri: durum.file.uri }}
          allowingReadAccessToURL={durum.file.uri}
          allowFileAccess
          onError={() => setOnizlemeYok(true)}
        />
      ) : null}

      {hazir && !durum.html && durum.tur === 'image' && !onizlemeYok ? (
        <ScrollView
          style={{ flex: 1 }}
          contentContainerStyle={styles.orta}
          maximumZoomScale={6}
          minimumZoomScale={1}
          bouncesZoom
          centerContent
          showsHorizontalScrollIndicator={false}
          showsVerticalScrollIndicator={false}>
          <Image
            testID="viewer-image"
            source={{ uri: durum.file.uri }}
            style={{ width, height: height * 0.8 }}
            contentFit="contain"
            accessibilityLabel={`${kaynak.title}, tam ekran. Yakınlaştırmak için iki parmağınızla açın.`}
            onError={() => setOnizlemeYok(true)}
          />
        </ScrollView>
      ) : null}
    </SafeAreaView>
  );
}

const styles = StyleSheet.create({
  orta: { flexGrow: 1, flex: 1, alignItems: 'center', justifyContent: 'center' },
});

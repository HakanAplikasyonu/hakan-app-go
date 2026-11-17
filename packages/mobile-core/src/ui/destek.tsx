// Destek ekranı (#387): ClickFix Web Widget'ı oturumu taşıyan güvenli bir
// WebView'de açar (ClickFix "Mobil: webview" kılavuzu, köprü sürümü 1).
// Widget kimliği yoksa ekran açılmaz; çağıran yedek mesajı gösterir.
import { useMemo, useRef, useState } from 'react';
import { ActivityIndicator, Linking, StyleSheet, View } from 'react-native';
import { WebView, type WebViewMessageEvent } from 'react-native-webview';

import {
  bridgeScript,
  embedOrigin,
  forgetOtherUserScript,
  embedPageUrl,
  isAllowedNavigation,
  parseBridgeMessage,
  visitorCommands,
  type SupportIdentity,
  type SupportVisitor,
} from '../support/clickfix.ts';
import { useColors } from './theme.ts';
import { Button, Notice, Text } from './ui.tsx';

export function ClickFixDestek({
  widgetId,
  embedUrl,
  locale = 'tr',
  userKey,
  visitor,
  identity = null,
  onClose,
  onUnread,
}: {
  widgetId: string;
  /** Barındırılan sayfa; boş = https://getclickfix.com/widget-embed.html */
  embedUrl?: string;
  locale?: string;
  /**
   * Oturumdaki kullanıcı (ve kapsam). Değişince sayfa açılmadan ClickFix'in
   * bu cihazdaki ziyaretçisi silinir: paylaşılan cihazda başka hesabın
   * konuşmaları görünmez (çıkışta widget kapalı olduğu için reset gidemez).
   */
  userKey: string;
  visitor: SupportVisitor;
  /** Sunucuda imzalanmış kimlik (user_hash); yoksa doğrulanmamış ziyaretçi. */
  identity?: SupportIdentity | null;
  onClose: () => void;
  onUnread?: (count: number) => void;
}) {
  const c = useColors();
  // WebView<P = undefined> would make the props `never`; give P explicitly.
  const ref = useRef<WebView<object>>(null);
  const [failed, setFailed] = useState(false);
  const [attempt, setAttempt] = useState(0);
  const uri = useMemo(() => embedPageUrl(embedUrl ?? '', widgetId, { locale }), [embedUrl, widgetId, locale]);
  const origin = useMemo(() => embedOrigin(embedUrl ?? ''), [embedUrl]);

  const onMessage = (e: WebViewMessageEvent) => {
    const msg = parseBridgeMessage(e.nativeEvent.data);
    if (!msg) return;
    if (msg.type === 'ready') {
      for (const cmd of visitorCommands(visitor, identity)) ref.current?.injectJavaScript(bridgeScript(cmd));
    } else if (msg.type === 'link') {
      // Yalnız https; sistem tarayıcısında açılır, WebView içinde değil.
      Linking.openURL(msg.url).catch(() => {});
    } else if (msg.type === 'close') {
      onClose();
    } else if (msg.type === 'unread') {
      onUnread?.(msg.count);
    }
  };

  if (failed) {
    return (
      <View style={[styles.center, { backgroundColor: c.background }]}>
        <Notice>Destek penceresi yüklenemedi. Bağlantınızı kontrol edip yeniden deneyin.</Notice>
        <Button
          title="Yeniden dene"
          onPress={() => {
            setFailed(false);
            setAttempt((n) => n + 1);
          }}
        />
      </View>
    );
  }

  return (
    <WebView
      key={attempt}
      ref={ref}
      source={{ uri }}
      style={{ flex: 1, backgroundColor: c.background }}
      originWhitelist={[origin]}
      onShouldStartLoadWithRequest={(r) => isAllowedNavigation(r.url, origin)}
      onMessage={onMessage}
      injectedJavaScriptBeforeContentLoaded={forgetOtherUserScript(userKey)}
      onError={() => setFailed(true)}
      onHttpError={(e) => {
        if (e.nativeEvent.statusCode >= 500) setFailed(true);
      }}
      startInLoadingState
      renderLoading={() => (
        <View style={[styles.center, StyleSheet.absoluteFill, { backgroundColor: c.background }]}>
          <ActivityIndicator color={c.brand} />
          <Text tone="muted">Destek yükleniyor…</Text>
        </View>
      )}
      javaScriptEnabled
      domStorageEnabled
      sharedCookiesEnabled={false}
      thirdPartyCookiesEnabled={false}
      allowFileAccess={false}
      allowFileAccessFromFileURLs={false}
      allowUniversalAccessFromFileURLs={false}
      setSupportMultipleWindows={false}
      allowsBackForwardNavigationGestures={false}
      accessibilityLabel="Destek"
    />
  );
}

const styles = StyleSheet.create({
  center: { flex: 1, alignItems: 'center', justifyContent: 'center', gap: 12, padding: 24 },
});

/**
 * Root screens both apps show (#202, #295): the render-error screen (the
 * expo-router ErrorBoundary) and the launch-without-connection screen. The
 * app passes its own logo mark and error reporter.
 */
import { useEffect, useState, type ReactNode } from 'react';
import { View } from 'react-native';
import { SafeAreaProvider } from 'react-native-safe-area-context';

import { HataDurumu } from './hakan.tsx';
import { Button, Screen } from './ui.tsx';

/**
 * Render errors anywhere below the root: reported when the app's reporter is
 * on, and the user can try again instead of seeing a blank screen.
 */
export function RootErrorScreen({
  error,
  retry,
  mark,
  report,
}: {
  error: Error;
  retry: () => Promise<void> | void;
  mark: ReactNode;
  report: (error: unknown, context?: Record<string, string>) => void;
}) {
  useEffect(() => {
    report(error, { boundary: 'root' });
  }, [error, report]);
  return (
    <SafeAreaProvider>
      <Screen style={{ justifyContent: 'center', alignItems: 'center', gap: 16 }}>
        {mark}
        {/* #332: ekran çizilirken düşen hata; kayıtlı veriye dokunulmaz. */}
        <HataDurumu
          baslik="Bu ekran açılamadı"
          aciklama="Ekran açılırken beklenmeyen bir hata oluştu. Tekrar deneyebilirsin."
          verilerGuvende
          kod={error.name !== 'Error' ? error.name : undefined}
          eylem={<Button title="Tekrar dene" onPress={retry} />}
        />
      </Screen>
    </SafeAreaProvider>
  );
}

/** Launch without a connection: the session is kept; retry checks again. */
export function OfflineScreen({ mark, retry }: { mark: ReactNode; retry: () => Promise<unknown> }) {
  const [busy, setBusy] = useState(false);
  return (
    <Screen style={{ justifyContent: 'center', alignItems: 'center', gap: 16 }}>
      {mark}
      {/* #332: geçici hata; oturum ve telefonda bekleyenler saklı. */}
      <HataDurumu
        baslik="Sunucuya ulaşılamıyor"
        aciklama="İnternet bağlantını kontrol edip tekrar dene; oturumun açık kalır."
        verilerGuvende
        kod="NETWORK_UNAVAILABLE"
        eylem={
          <Button
            title="Tekrar dene"
            loading={busy}
            onPress={() => {
              setBusy(true);
              retry().finally(() => setBusy(false));
            }}
          />
        }
      />
    </Screen>
  );
}

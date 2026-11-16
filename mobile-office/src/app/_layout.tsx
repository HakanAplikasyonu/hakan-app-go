// Kaynak: mobile/src/app/_layout.tsx (#236); ofis akışına uyarlandı.
import { OfflineScreen, RootErrorScreen } from '@hakan/mobile-core/screens';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { DarkTheme, DefaultTheme, Stack, ThemeProvider, type ErrorBoundaryProps } from 'expo-router';
import * as SplashScreen from 'expo-splash-screen';
import { StatusBar } from 'expo-status-bar';
import { useEffect, useMemo, useState } from 'react';
import { ActivityIndicator, View } from 'react-native';
import { SafeAreaProvider } from 'react-native-safe-area-context';

import { BrandMark } from '@/components/brand-mark';
import { AuthProvider, useAuth } from '@/core/auth/auth-provider';
import { initSentry, reportError, wrapRoot } from '@/core/observability/sentry';
import { installForegroundHandler } from '@/core/push/push';
import { useNotificationRouting } from '@/core/push/use-notification-routing';
import { useColors, useScheme } from '@/core/theme';

initSentry();
SplashScreen.preventAutoHideAsync().catch(() => {});
installForegroundHandler();

/**
 * Kökün altındaki render hataları (#241; kaynak mobile/src/app/_layout.tsx,
 * #202): Sentry açıksa raporlanır, kullanıcı boş ekran yerine yeniden dener.
 */
export function ErrorBoundary({ error, retry }: ErrorBoundaryProps) {
  return <RootErrorScreen error={error} retry={retry} mark={<BrandMark size={64} />} report={reportError} />;
}

/** Bağlantısız açılış: oturum korunur, "Tekrar dene" yeniden denetler. */
function Offline() {
  const { retry } = useAuth();
  return <OfflineScreen mark={<BrandMark size={64} />} retry={retry} />;
}

function RootNavigator() {
  const { status } = useAuth();
  useNotificationRouting();
  const c = useColors();
  const scheme = useScheme();
  const navTheme = useMemo(() => {
    const base = scheme === 'dark' ? DarkTheme : DefaultTheme;
    return {
      ...base,
      colors: { ...base.colors, primary: c.brand, background: c.background, card: c.surface, text: c.text, border: c.border },
    };
  }, [scheme, c]);

  useEffect(() => {
    if (status !== 'booting') SplashScreen.hideAsync().catch(() => {});
  }, [status]);

  if (status === 'offline') return <Offline />;

  if (status === 'booting') {
    return (
      <View style={{ flex: 1, alignItems: 'center', justifyContent: 'center', backgroundColor: c.background }}>
        <BrandMark size={96} />
        <ActivityIndicator color={c.brand} style={{ position: 'absolute', bottom: '22%' }} />
      </View>
    );
  }

  return (
    <ThemeProvider value={navTheme}>
      <StatusBar style={scheme === 'dark' ? 'light' : 'dark'} />
      <Stack screenOptions={{ headerShown: false, contentStyle: { backgroundColor: c.background } }}>
        <Stack.Protected guard={status === 'signedOut'}>
          <Stack.Screen name="login" />
        </Stack.Protected>
        <Stack.Protected guard={status === 'denied'}>
          <Stack.Screen name="denied" />
        </Stack.Protected>
        <Stack.Protected guard={status === 'selectOffice'}>
          <Stack.Screen name="select-office" />
        </Stack.Protected>
        <Stack.Protected guard={status === 'ready'}>
          <Stack.Screen name="(app)" />
        </Stack.Protected>
      </Stack>
    </ThemeProvider>
  );
}

function RootLayout() {
  const [queryClient] = useState(() => new QueryClient({ defaultOptions: { queries: { retry: 1, staleTime: 30_000 } } }));
  return (
    <SafeAreaProvider>
      <QueryClientProvider client={queryClient}>
        <AuthProvider>
          <RootNavigator />
        </AuthProvider>
      </QueryClientProvider>
    </SafeAreaProvider>
  );
}

export default wrapRoot(RootLayout);

import { OfflineScreen, RootErrorScreen } from '@hakan/mobile-core/screens';
import { hakanFonts } from '@hakan/mobile-core/ui/fonts';
import { loadThemePreference } from '@hakan/mobile-core/ui/theme';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { useFonts } from 'expo-font';
import { DarkTheme, DefaultTheme, Stack, ThemeProvider, type ErrorBoundaryProps } from 'expo-router';
import * as SplashScreen from 'expo-splash-screen';
import { StatusBar } from 'expo-status-bar';
import { useEffect, useMemo, useState } from 'react';
import { ActivityIndicator, View } from 'react-native';
import { SafeAreaProvider } from 'react-native-safe-area-context';

import { BrandMark } from '@/components/brand-mark';
import { AuthProvider, useAuth } from '@/core/auth/auth-provider';
import { installForegroundHandler } from '@/core/push/push';
import { useNotificationRouting } from '@/core/push/use-notification-routing';
import { initSentry, reportError, wrapRoot } from '@/core/observability/sentry';
import { useColors, useScheme } from '@/core/theme';

initSentry();
SplashScreen.preventAutoHideAsync().catch(() => {});
installForegroundHandler();

/**
 * Render errors anywhere below the root (#202): reported to Sentry when it is
 * on, and the user can try again instead of seeing a blank screen.
 */
export function ErrorBoundary({ error, retry }: ErrorBoundaryProps) {
  return <RootErrorScreen error={error} retry={retry} mark={<BrandMark size={64} />} report={reportError} />;
}

/** Launch without a connection (#202): the session is kept; retry checks again. */
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
          <Stack.Screen name="forgot-password" />
        </Stack.Protected>
        <Stack.Protected guard={status === 'selectTaxpayer'}>
          <Stack.Screen name="select-taxpayer" />
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
  // Arayüz v1 (#324): Inter / Inter Tight / JetBrains Mono ve kayıtlı tema
  // (Ayarlar → Görünüm). Yazı tipleri yüklenene kadar açılış ekranı kalır;
  // yüklenemezse sistem yazı tipiyle devam edilir.
  const [fontsLoaded, fontError] = useFonts(hakanFonts);
  useEffect(() => {
    loadThemePreference().catch(() => {});
  }, []);
  if (!fontsLoaded && !fontError) return null;
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

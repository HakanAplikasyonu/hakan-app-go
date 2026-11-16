// Kaynak: mobile/src/core/push/push.ts (#236'da kopyalandı; ortak paket ayrı issue).
/**
 * Expo push: permission, token and device registration (`/v1/push-devices`).
 *
 * The native module is loaded lazily so the app keeps running where push is not
 * available (web, Expo Go on Android, simulators without APNs): every call here
 * is then a quiet no-op.
 */
import Constants, { ExecutionEnvironment } from 'expo-constants';
import { getLocales } from 'expo-localization';
import { Platform } from 'react-native';

import { api } from '../api/client';
import { secure, StorageKeys } from '../storage';
import { notificationUrl } from './links';

type NotificationsModule = typeof import('expo-notifications');

const isExpoGo = Constants.executionEnvironment === ExecutionEnvironment.StoreClient;

let mod: NotificationsModule | null | undefined;

export function notificationsModule(): NotificationsModule | null {
  if (mod !== undefined) return mod;
  if (Platform.OS === 'web' || (isExpoGo && Platform.OS === 'android')) {
    mod = null;
    return mod;
  }
  try {
    // eslint-disable-next-line @typescript-eslint/no-require-imports -- lazy: must not load in Expo Go on Android
    mod = require('expo-notifications') as NotificationsModule;
  } catch {
    mod = null;
  }
  return mod;
}

export type PushPermission = 'granted' | 'denied' | 'undetermined' | 'unavailable';

export async function getPushPermission(): Promise<PushPermission> {
  const N = notificationsModule();
  if (!N || isExpoGo) return 'unavailable';
  try {
    const res = await N.getPermissionsAsync();
    if (res.granted || res.ios?.status === N.IosAuthorizationStatus.PROVISIONAL) return 'granted';
    return res.canAskAgain === false || res.status === 'denied' ? 'denied' : 'undetermined';
  } catch {
    return 'unavailable';
  }
}

async function ensureAndroidChannel(N: NotificationsModule) {
  if (Platform.OS !== 'android') return;
  // Android 13+ only shows the permission prompt once a channel exists.
  await N.setNotificationChannelAsync('default', { name: 'Hakan Ofis', importance: N.AndroidImportance.HIGH });
}

function projectId(): string | undefined {
  const extra = Constants.expoConfig?.extra as { eas?: { projectId?: string } } | undefined;
  return extra?.eas?.projectId ?? Constants.easConfig?.projectId;
}

function deviceInfo() {
  const version = Constants.expoConfig?.version ?? '';
  const build = Platform.OS === 'ios' ? Constants.expoConfig?.ios?.buildNumber : Constants.expoConfig?.android?.versionCode;
  return {
    platform: Platform.OS as 'ios' | 'android',
    locale: getLocales()[0]?.languageTag ?? 'tr',
    app_version: (build ? `${version} (${build})` : version).slice(0, 32),
    device_name: (Constants.deviceName ?? '').slice(0, 120) || undefined,
  };
}

export type RegisterResult = 'registered' | 'unavailable' | 'denied' | 'failed';

let inFlight: Promise<RegisterResult> | null = null;

/** Registers this device for the signed-in user. Never prompts. */
export function registerPushDevice(): Promise<RegisterResult> {
  inFlight ??= doRegister().finally(() => {
    inFlight = null;
  });
  return inFlight;
}

async function doRegister(): Promise<RegisterResult> {
  const N = notificationsModule();
  const permission = await getPushPermission();
  if (!N || permission === 'unavailable') return 'unavailable';
  if (permission !== 'granted') return 'denied';
  const id = projectId();
  if (!id) return 'unavailable'; // no EAS project yet (eas init)
  try {
    await ensureAndroidChannel(N);
    const token = (await N.getExpoPushTokenAsync({ projectId: id })).data;
    const previous = await secure.get(StorageKeys.pushToken);
    await api.post('/v1/push-devices', { token, ...deviceInfo() });
    await secure.set(StorageKeys.pushToken, token);
    if (previous && previous !== token) {
      // Token rotated: drop the stale one so the backend stops sending to it.
      api.delete(`/v1/push-devices/${encodeURIComponent(previous)}`).catch(() => {});
    }
    return 'registered';
  } catch {
    return 'failed';
  }
}

/** Asks for permission if it was never asked, then registers. */
export async function enablePush(): Promise<RegisterResult> {
  const N = notificationsModule();
  let permission = await getPushPermission();
  if (!N || permission === 'unavailable') return 'unavailable';
  if (permission === 'undetermined') {
    await ensureAndroidChannel(N);
    const res = await N.requestPermissionsAsync({ ios: { allowAlert: true, allowBadge: true, allowSound: true } });
    permission = res.granted ? 'granted' : 'denied';
  }
  if (permission !== 'granted') return 'denied';
  return registerPushDevice();
}

/** Removes this device from the user; call before the tokens are dropped. Never throws. */
export async function unregisterPushDevice() {
  const token = await secure.get(StorageKeys.pushToken);
  if (!token) return;
  try {
    await api.delete(`/v1/push-devices/${encodeURIComponent(token)}`);
  } catch {
    // The server also drops tokens Expo reports as DeviceNotRegistered.
  } finally {
    await secure.remove(StorageKeys.pushToken);
  }
}

/** Foreground notifications show as banners too. */
export function installForegroundHandler() {
  const N = notificationsModule();
  N?.setNotificationHandler({
    handleNotification: async () => ({
      shouldShowBanner: true,
      shouldShowList: true,
      shouldPlaySound: true,
      shouldSetBadge: true,
    }),
  });
}

type ResponseHandler = (url: string, id: string) => void;

/**
 * Bildirime dokunuş (#241; kaynak mobile/src/core/push/push.ts, #201): uygulama
 * açıkken gelen ve uygulamayı soğuk başlatan dokunuşlar. Aynı dokunuş bir kez
 * işlenir.
 */
export function subscribeNotificationOpens(onOpen: ResponseHandler): () => void {
  const N = notificationsModule();
  if (!N) return () => {};
  const seen = new Set<string>();
  const handle = (r: import('expo-notifications').NotificationResponse | null | undefined) => {
    if (!r) return;
    const id = r.notification.request.identifier;
    if (seen.has(id)) return;
    seen.add(id);
    const url = notificationUrl(r.notification.request.content.data);
    if (url) onOpen(url, id);
  };
  const sub = N.addNotificationResponseReceivedListener(handle);
  N.getLastNotificationResponseAsync()
    .then((r) => {
      handle(r);
      // Bir sonraki açılışta aynı dokunuş yeniden açılmasın.
      N.clearLastNotificationResponseAsync?.().catch(() => {});
    })
    .catch(() => {});
  return () => sub.remove();
}

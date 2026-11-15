import * as SecureStore from 'expo-secure-store';
import { Platform } from 'react-native';

/** Keys of values kept on the device. */
export const StorageKeys = {
  session: 'hakan.session',
  taxpayer: 'hakan.taxpayer',
  pushToken: 'hakan.push-token',
} as const;

/**
 * Secure storage (Keychain / Keystore). The web build (used only for screen
 * checks in development) falls back to localStorage.
 */
export const secure = {
  async get(key: string): Promise<string | null> {
    if (Platform.OS === 'web') return globalThis.localStorage?.getItem(key) ?? null;
    return SecureStore.getItemAsync(key);
  },
  async set(key: string, value: string): Promise<void> {
    if (Platform.OS === 'web') {
      globalThis.localStorage?.setItem(key, value);
      return;
    }
    await SecureStore.setItemAsync(key, value);
  },
  async remove(key: string): Promise<void> {
    if (Platform.OS === 'web') {
      globalThis.localStorage?.removeItem(key);
      return;
    }
    await SecureStore.deleteItemAsync(key);
  },
};

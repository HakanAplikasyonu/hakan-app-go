/**
 * Tema (arayüz v1, #324). Dört tema: m3-light, m3-dark, n3-light, n3-dark.
 * Varsayılan M3 + sistemi izle; seçim cihazda saklanır (Ayarlar → Görünüm).
 *
 * - `useTheme()`: yeni token'lar (`colors`, `tokens`, tema adı).
 * - `useColors()`: eski `Palette` adları, yeni token'lara bağlı (mevcut ekranlar
 *   değişmeden yeni renkleri alır).
 */
import { useSyncExternalStore } from 'react';
import { useColorScheme } from 'react-native';

import { secure } from '../secure-store.ts';
import { paletteFrom, type Palette } from './palette.ts';
import {
  THEME_PREFERENCE_KEY,
  defaultThemePreference,
  parseThemePreference,
  resolveThemeName,
  serializeThemePreference,
  type ThemePreference,
} from './theme-pref.ts';
import { colors, tokens, type Colors, type ThemeName } from './tokens.ts';

export { colors, tokens, type Colors, type ThemeName };
export type { ThemeMode, ThemePalette, ThemePreference } from './theme-pref.ts';

let current: ThemePreference = defaultThemePreference;
const listeners = new Set<() => void>();

function subscribe(listener: () => void) {
  listeners.add(listener);
  return () => listeners.delete(listener);
}

/** Uygulama açılırken bir kez çağrılır (kök _layout). */
export async function loadThemePreference(): Promise<void> {
  try {
    current = parseThemePreference(await secure.get(THEME_PREFERENCE_KEY));
  } catch {
    current = defaultThemePreference;
  }
  listeners.forEach((l) => l());
}

export function setThemePreference(next: Partial<ThemePreference>): void {
  current = { ...current, ...next };
  listeners.forEach((l) => l());
  secure.set(THEME_PREFERENCE_KEY, serializeThemePreference(current)).catch(() => {});
}

export function useThemePreference(): ThemePreference {
  return useSyncExternalStore(subscribe, () => current, () => current);
}

export function useTheme(): { name: ThemeName; scheme: 'light' | 'dark'; colors: Colors; tokens: typeof tokens } {
  const pref = useThemePreference();
  const name = resolveThemeName(pref, useColorScheme());
  return { name, scheme: name.endsWith('-dark') ? 'dark' : 'light', colors: colors[name], tokens };
}

export function useScheme(): 'light' | 'dark' {
  return useTheme().scheme;
}

export function useColors(): Palette {
  return paletteFrom(colors[useTheme().name]);
}

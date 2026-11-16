/**
 * Tema tercihi (arayüz v1, #324): palet (m3 | n3) × kip (açık | koyu | sistemi
 * izle). Saf modül; React Native içe aktarmaz, birim testi node ile koşar.
 */
import type { ThemeName } from './tokens.ts';

export type ThemePalette = 'm3' | 'n3';
export type ThemeMode = 'light' | 'dark' | 'system';
export type ThemePreference = { palette: ThemePalette; mode: ThemeMode };

/** Varsayılan: M3, sistemi izle (docs/tasarim/README.md, Temalar). */
export const defaultThemePreference: ThemePreference = { palette: 'm3', mode: 'system' };

export const THEME_PREFERENCE_KEY = 'hakan.theme';

export function resolveThemeName(pref: ThemePreference, system: 'light' | 'dark' | null | undefined): ThemeName {
  const mode = pref.mode === 'system' ? (system === 'dark' ? 'dark' : 'light') : pref.mode;
  return `${pref.palette}-${mode}`;
}

export function parseThemePreference(raw: string | null | undefined): ThemePreference {
  if (!raw) return defaultThemePreference;
  try {
    const v = JSON.parse(raw) as Partial<ThemePreference>;
    const palette: ThemePalette = v.palette === 'n3' ? 'n3' : 'm3';
    const mode: ThemeMode = v.mode === 'light' || v.mode === 'dark' ? v.mode : 'system';
    return { palette, mode };
  } catch {
    return defaultThemePreference;
  }
}

export function serializeThemePreference(pref: ThemePreference): string {
  return JSON.stringify(pref);
}

/** < .6 düşük, < .9 orta, aksi yüksek; kural eşleşmesi her zaman yüksek. */
export type GuvenKaynagi = 'kural' | 'gecmis' | 'model' | 'girdi';
export type GuvenKademesi = 'dusuk' | 'orta' | 'yuksek';
export function guvenKademesi(deger: number, kaynak?: GuvenKaynagi): GuvenKademesi {
  if (kaynak === 'kural') return 'yuksek';
  if (deger < 0.6) return 'dusuk';
  if (deger < 0.9) return 'orta';
  return 'yuksek';
}

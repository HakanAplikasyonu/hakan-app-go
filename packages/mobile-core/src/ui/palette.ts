/**
 * Eski renk adları (D19 dönemi). Arayüz v1 (#324) ile değerler tokens.ts'ten
 * türetilir; yeni kod `useTheme().colors` kullanır.
 */
import { colors, type Colors } from './tokens.ts';

export type Palette = {
  background: string;
  surface: string;
  border: string;
  text: string;
  muted: string;
  primary: string;
  primaryText: string;
  brand: string;
  check: string;
  danger: string;
  success: string;
  warning: string;
};

const cache = new WeakMap<Colors, Palette>();

export function paletteFrom(c: Colors): Palette {
  let p = cache.get(c);
  if (!p) {
    p = {
      background: c.bg,
      surface: c.surface,
      border: c.line,
      text: c.text,
      muted: c.text3,
      primary: c.brand,
      primaryText: c.onBrand,
      brand: c.brandText,
      check: c.crystal.mid,
      danger: c.danger,
      success: c.ok,
      warning: c.warn,
    };
    cache.set(c, p);
  }
  return p;
}

export const palettes: Record<'light' | 'dark', Palette> = {
  light: paletteFrom(colors['m3-light']),
  dark: paletteFrom(colors['m3-dark']),
};

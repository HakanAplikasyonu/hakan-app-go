// Kaynak: mobile/src/core/theme.ts (#236'da kopyalandı; ortak paket ayrı issue).
import { useColorScheme } from 'react-native';

import { palettes, type Palette } from './brand';

export function useScheme(): 'light' | 'dark' {
  return useColorScheme() === 'dark' ? 'dark' : 'light';
}

export function useColors(): Palette {
  return palettes[useScheme()];
}

import { useColorScheme } from 'react-native';

import { palettes, type Palette } from './brand';

export function useScheme(): 'light' | 'dark' {
  return useColorScheme() === 'dark' ? 'dark' : 'light';
}

export function useColors(): Palette {
  return palettes[useScheme()];
}

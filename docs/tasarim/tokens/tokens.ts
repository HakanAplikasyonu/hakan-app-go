/**
 * HAKAN tasarım token'ları · v1 · Expo (mobile/, mobile-office/) için.
 * Hedef yer: packages/mobile-core/src/ui/theme.ts (palette.ts'in yerine).
 * tokens.css ile birebir aynı değerler; değişiklik iki dosyada birlikte yapılır.
 */
export type ThemeName = 'm3-light' | 'm3-dark' | 'n3-light' | 'n3-dark';

const base = {
  space: { 1: 4, 2: 8, 3: 12, 4: 16, 5: 20, 6: 24, 8: 32, 10: 40, 14: 56, 18: 72 },
  radius: { sm: 6, md: 10, lg: 14, xl: 22, sheet: 22 },
  font: {
    display: 'InterTight_700Bold',
    sans: 'Inter_400Regular',
    sansMedium: 'Inter_500Medium',
    sansSemi: 'Inter_600SemiBold',
    sansBold: 'Inter_700Bold',
    mono: 'JetBrainsMono_500Medium',
  },
  // Mobil gövde 15/22; Dynamic Type ile ölçeklenir (allowFontScaling açık kalır).
  type: {
    display: { size: 34, line: 38 },
    h1: { size: 26, line: 32 },
    h2: { size: 20, line: 26 },
    body: { size: 15, line: 22 },
    small: { size: 13, line: 18 },
    caption: { size: 11, line: 14, letterSpacing: 0.6 },
  },
  motion: { fast: 120, base: 200, slow: 320, spring: { damping: 18, stiffness: 220 } },
  hitSlop: 44,
} as const;

const m3Light = {
  bg: '#F3F3F5', surface: '#FFFFFF', surface2: '#F8F8FA', surface3: '#EDEDF1', line: '#E3E3E9', lineStrong: '#CACAD3',
  text: '#0A0A0A', text2: '#4A4A55', text3: '#6B6B76',
  brand: '#2448FF', onBrand: '#FFFFFF', brandText: '#2448FF', brandInk: '#102290', brandSoft: '#CFD8FF', brandTint: '#EEF1FF',
  crystal: { light: '#CFD8FF', mid: '#2448FF', dark: '#102290' },
  ok: '#0E7A4E', okTint: '#E3F4EC', warn: '#9A5800', warnTint: '#FFF1DC', danger: '#C0261C', dangerTint: '#FDE7E5',
  focus: '#2448FF',
};
const m3Dark: typeof m3Light = {
  bg: '#0A0A0A', surface: '#131316', surface2: '#1A1A1E', surface3: '#232329', line: '#26262C', lineStrong: '#3A3A43',
  text: '#F3F3F5', text2: '#B4B4BE', text3: '#8C8C97',
  brand: '#2448FF', onBrand: '#FFFFFF', brandText: '#93A5FF', brandInk: '#CFD8FF', brandSoft: '#1E2C8F', brandTint: '#151B45',
  crystal: { light: '#CFD8FF', mid: '#2448FF', dark: '#102290' },
  ok: '#4FD39A', okTint: '#0F2A1F', warn: '#FFB54D', warnTint: '#2E2210', danger: '#FF7A70', dangerTint: '#331614',
  focus: '#93A5FF',
};
const n3Light: typeof m3Light = {
  ...m3Light,
  brand: '#01FF11', onBrand: '#0A0A0A', brandText: '#067A12', brandInk: '#0A0A0A', brandSoft: '#D4D3D8', brandTint: '#E6FFE8',
  crystal: { light: '#D4D3D8', mid: '#01FF11', dark: '#0A0A0A' },
  ok: '#067A12', okTint: '#E6FFE8', focus: '#0A0A0A',
};
const n3Dark: typeof m3Light = {
  ...m3Dark,
  brand: '#01FF11', onBrand: '#0A0A0A', brandText: '#01FF11', brandInk: '#F3F3F5', brandSoft: '#2A2A30', brandTint: '#0D2410',
  crystal: { light: '#D4D3D8', mid: '#01FF11', dark: '#F3F3F5' },
  ok: '#01FF11', okTint: '#0D2410', focus: '#01FF11',
};

export const colors: Record<ThemeName, typeof m3Light> = {
  'm3-light': m3Light, 'm3-dark': m3Dark, 'n3-light': n3Light, 'n3-dark': n3Dark,
};

export const tokens = base;
export type Colors = typeof m3Light;

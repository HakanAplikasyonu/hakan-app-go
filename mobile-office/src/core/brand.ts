/**
 * Uygulama kimliği (D19, docs/brand). Değerler frontend/src/config/brand.ts ve
 * frontend/src/styles/theme.css ile aynı tutulur.
 */
export const brand = {
  name: 'Hakan Ofis',
  tagline: 'Mükellefleriniz, inceleme kuyruğunuz ve uyarılarınız cebinizde',
} as const;

// Renk paleti iki uygulamada ortak (@hakan/mobile-core, #295).
export { palettes, type Palette } from '@hakan/mobile-core/ui/palette';

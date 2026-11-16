// Kaynak: mobile/src/core/brand.ts (#236'da kopyalandı; ortak paket ayrı issue).
/**
 * Uygulama kimliği (D19, docs/brand). Değerler frontend/src/config/brand.ts ve
 * frontend/src/styles/theme.css ile aynı tutulur.
 */
export const brand = {
  name: 'Hakan Ofis',
  tagline: 'Mükellefleriniz, inceleme kuyruğunuz ve uyarılarınız cebinizde',
} as const;

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

export const palettes: Record<'light' | 'dark', Palette> = {
  light: {
    background: '#F8FAFC',
    surface: '#FFFFFF',
    border: '#E2E8F0',
    text: '#0F172A',
    muted: '#64748B',
    primary: '#0F172A',
    primaryText: '#F8FAFC',
    brand: '#1447E6',
    check: '#5B8CFF',
    danger: '#E7000B',
    success: '#009689',
    warning: '#F54900',
  },
  dark: {
    background: '#020617',
    surface: '#0F172A',
    border: '#1E293B',
    text: '#F8FAFC',
    muted: '#94A3B8',
    primary: '#F8FAFC',
    primaryText: '#0F172A',
    brand: '#5B8CFF',
    check: '#1447E6',
    danger: '#FB2C36',
    success: '#00BBA7',
    warning: '#FF6900',
  },
};

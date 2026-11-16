import type { SekmeRenkleri } from '@hakan/mobile-core/ui/sekme-cubugu';
import { useTheme } from '@hakan/mobile-core/ui/theme';

/** Sekme çubuğu ve ray renkleri, etkin temadan (useTheme). */
export function useSekmeRenkleri(): SekmeRenkleri & { bg: string; brand: string; onBrand: string } {
  const { colors: c } = useTheme();
  return {
    bg: c.bg,
    surface: c.surface,
    line: c.line,
    text3: c.text3,
    brandText: c.brandText,
    brandTint: c.brandTint,
    brand: c.brand,
    onBrand: c.onBrand,
    danger: c.danger,
    onDanger: c.surface,
  };
}

import { useTheme } from '@hakan/mobile-core/ui/theme';
import type { SekmeRenkleri } from '@hakan/mobile-core/ui/sekme-cubugu';

/** Sekme çubuğu renkleri, etkin temadan (useTheme). */
export function useSekmeRenkleri(): SekmeRenkleri & { bg: string } {
  const { colors: c } = useTheme();
  return { bg: c.bg, surface: c.surface, line: c.line, text3: c.text3, brandText: c.brandText, brandTint: c.brandTint, danger: c.danger, onDanger: c.surface };
}

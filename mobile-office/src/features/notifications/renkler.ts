import type { SekmeRenkleri } from '@hakan/mobile-core/ui/sekme-cubugu';

import { useColors } from '@/core/theme';

/** Sekme çubuğu renkleri, uygulamanın temasından. */
export function useSekmeRenkleri(): SekmeRenkleri & { bg: string } {
  const c = useColors();
  return { bg: c.background, surface: c.surface, line: c.border, text3: c.muted, brandText: c.brand, brandTint: c.background, danger: c.danger, onDanger: '#FFFFFF' };
}

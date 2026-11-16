// Kaynak: mobile/src/components/brand-mark.tsx (#236'da kopyalandı; ortak paket ayrı issue).
import Svg, { Path, Rect } from 'react-native-svg';

import { useScheme } from '@/core/theme';

/**
 * Logo işareti, ofis uygulaması rengiyle (#242; assets/src/office-mark.svg):
 * mavi zemin, mükellef uygulamasından ayırt edilsin.
 */
export function BrandMark({ size = 56 }: { size?: number }) {
  const dark = useScheme() === 'dark';
  const tile = dark ? '#5B8CFF' : '#1447E6';
  const bars = '#FFFFFF';
  const check = '#0F172B';
  return (
    <Svg width={size} height={size} viewBox="0 0 64 64" accessibilityLabel="Hakan Ofis">
      <Rect width={64} height={64} rx={15} fill={tile} />
      <Rect x={15} y={14} width={8} height={36} rx={4} fill={bars} />
      <Rect x={41} y={14} width={8} height={36} rx={4} fill={bars} />
      <Path d="M21 33 L29 41 L43 25" stroke={check} strokeWidth={6} fill="none" strokeLinecap="round" strokeLinejoin="round" />
    </Svg>
  );
}

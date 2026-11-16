// Kaynak: mobile/src/components/brand-mark.tsx (#236'da kopyalandı; ortak paket ayrı issue).
import Svg, { Path, Rect } from 'react-native-svg';

import { useScheme } from '@/core/theme';

/** Logo işareti (docs/brand/logo-mark.svg; koyu zeminde logo-mark-dark.svg). */
export function BrandMark({ size = 56 }: { size?: number }) {
  const dark = useScheme() === 'dark';
  const tile = dark ? '#FFFFFF' : '#0F172B';
  const bars = dark ? '#0F172B' : '#FFFFFF';
  const check = dark ? '#1447E6' : '#5B8CFF';
  return (
    <Svg width={size} height={size} viewBox="0 0 64 64" accessibilityLabel="Hakan">
      <Rect width={64} height={64} rx={15} fill={tile} />
      <Rect x={15} y={14} width={8} height={36} rx={4} fill={bars} />
      <Rect x={41} y={14} width={8} height={36} rx={4} fill={bars} />
      <Path d="M21 33 L29 41 L43 25" stroke={check} strokeWidth={6} fill="none" strokeLinecap="round" strokeLinejoin="round" />
    </Svg>
  );
}

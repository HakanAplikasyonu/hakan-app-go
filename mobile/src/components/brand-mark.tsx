import Svg, { Polygon, Rect } from 'react-native-svg';

/** M3 kristal yüzleri: açık (mükellef), orta (beyanname), koyu (mali müşavir). */
const POLY = [
  '0,86 28,59 28,198 0,198',
  '28,59 56,86 56,198 28,198',
  '63,36 91,0 91,198 63,198',
  '91,0 119,36 119,198 91,198',
  '125,86 153,59 153,198 125,198',
  '153,59 181,86 181,198 153,198',
];
const FILL = ['#CFD8FF', '#2448FF', '#2448FF', '#102290', '#102290', '#CFD8FF'];

/**
 * Uygulama ikonu karosu (docs/brand/icons/app-icon-rounded.svg): M3 "Taç
 * kristal" işareti, mükellef uygulamasında siyah zemin. Açık ve koyu temada aynı.
 */
export function BrandMark({ size = 56 }: { size?: number }) {
  // Tuval: 64 px karoda 34×37 px işaret, ortalı.
  const s = 37 / 198;
  return (
    <Svg width={size} height={size} viewBox="0 0 64 64" accessibilityLabel="Hakan">
      <Rect width={64} height={64} rx={15} fill="#0A0A0A" />
      {POLY.map((p, i) => (
        <Polygon key={p} points={p} fill={FILL[i]} transform={`translate(${(64 - 181 * s) / 2} ${(64 - 37) / 2}) scale(${s})`} />
      ))}
    </Svg>
  );
}

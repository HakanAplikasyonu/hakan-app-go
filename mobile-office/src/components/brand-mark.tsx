// Kaynak: mobile/src/components/brand-mark.tsx; ofis karosu mavi (#242).
import Svg, { Polygon, Rect } from 'react-native-svg';

const POLY = [
  '0,86 28,59 28,198 0,198',
  '28,59 56,86 56,198 28,198',
  '63,36 91,0 91,198 63,198',
  '91,0 119,36 119,198 91,198',
  '125,86 153,59 153,198 125,198',
  '153,59 181,86 181,198 153,198',
];
/** Mavi zeminde orta kristal beyaz olur (tuval Main.dc.html, uygulama ikonları). */
const FILL = ['#CFD8FF', '#FFFFFF', '#FFFFFF', '#102290', '#102290', '#CFD8FF'];

/**
 * Uygulama ikonu karosu (docs/brand/icons/app-icon-rounded-office.svg): M3
 * işareti, müşavir uygulamasında mavi zemin; ana ekranda mükellef
 * uygulamasıyla karışmaz.
 */
export function BrandMark({ size = 56 }: { size?: number }) {
  const s = 37 / 198;
  return (
    <Svg width={size} height={size} viewBox="0 0 64 64" accessibilityLabel="Hakan Ofis">
      <Rect width={64} height={64} rx={15} fill="#2448FF" />
      {POLY.map((p, i) => (
        <Polygon key={p} points={p} fill={FILL[i]} transform={`translate(${(64 - 181 * s) / 2} ${(64 - 37) / 2}) scale(${s})`} />
      ))}
    </Svg>
  );
}

import Svg, { Path } from 'react-native-svg';

/**
 * HAKAN ikon seti (tasarım: Kristal panosu, "İkon seti"). Lucide tabanlı
 * çizgiler, 24'lük ızgara, çizgi kalınlığı 1.75. Yollar tuvalden birebir
 * alındı (docs/tasarim/tuval/Kristal.dc.html ve mobil panolar). Yeni bağımlılık
 * gerektirmez (react-native-svg iki uygulamada da var).
 */
export const ikonYollari = {
  pano: 'M4 20V11l3-3 3 3v9M10 20V7l3-4 3 4v13M16 20v-9l3-3 3 3v9',
  kuyruk: 'M3 13l2.5-8h13L21 13v6H3z',
  mukellef: 'M16 20v-1.5a3.5 3.5 0 00-3.5-3.5h-5A3.5 3.5 0 004 18.5V20M10 11.5a3.5 3.5 0 100-7 3.5 3.5 0 000 7',
  gorev: 'M4 4h16v12H8l-4 4z',
  talep: 'M4 4h16v12H8l-4 4zM9 9h6M9 12h3',
  bildirim: 'M6 9a6 6 0 0112 0c0 6 2.5 8 2.5 8h-17S6 15 6 9M10 20a2.2 2.2 0 004 0',
  gecmis: 'M6 3h9l4 4v14l-2-1.2-2 1.2-2-1.2-2 1.2-2-1.2L6 21z',
  profil: 'M12 12a4 4 0 100-8 4 4 0 000 8M4 21a8 8 0 0116 0',
  fisCek: 'M4 8h3l2-3h6l2 3h3v12H4zM12 17a4 4 0 100-8 4 4 0 000 8',
  onayla: 'M4 12.5l5 5L20 6.5',
  kapat: 'M6 6l12 12M18 6L6 18',
  duzelt: 'M4 20h4L19 9l-4-4L4 16zM13.5 6.5l4 4',
  ileri: 'M5 12h14M13 6l6 6-6 6',
  sag: 'M9 6l6 6-6 6',
  geri: 'M15 6l-6 6 6 6',
  uyari: 'M12 3l9.5 17h-19zM12 10v4M12 17h.01',
  saat: 'M12 8v4l3 2M12 21a9 9 0 100-18 9 9 0 000 18',
  arac: 'M3 13h18M5 13l2-6h10l2 6v5H5z',
  ara: 'M11 18a7 7 0 100-14 7 7 0 000 14M20 20l-4-4',
  klasor: 'M3 6h6l2 2h10v11H3z',
  yukle: 'M12 15V3M7 8l5-5 5 5M4 15v4a2 2 0 002 2h12a2 2 0 002-2v-4',
  luca: 'M5 21h14M12 3v12M7 10l5 5 5-5M5 3h3M16 3h3',
  bulutYok: 'M2 2l20 20M5.8 5.8A6 6 0 004 10.5 4.5 4.5 0 006.5 19H17m3.6-2.3A4 4 0 0018 9.1h-.5A7 7 0 009.3 4.2',
  kilit: 'M6 11h12v10H6zM8.5 11V7.5a3.5 3.5 0 017 0V11',
  eposta: 'M3 5h18v14H3zM3 6l9 7 9-7',
} as const;

export type IkonAdi = keyof typeof ikonYollari;

/** Süs ikonudur: ekran okuyucudan gizlenir, anlamı yanındaki metin taşır. */
export function Ikon({ ad, boyut = 18, renk }: { ad: IkonAdi; boyut?: 16 | 18 | 22 | 24 | 28; renk: string }) {
  return (
    <Svg width={boyut} height={boyut} viewBox="0 0 24 24" accessibilityElementsHidden importantForAccessibility="no-hide-descendants">
      <Path d={ikonYollari[ad]} stroke={renk} strokeWidth={1.75} strokeLinecap="round" strokeLinejoin="round" fill="none" />
    </Svg>
  );
}

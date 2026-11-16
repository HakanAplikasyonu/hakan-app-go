/**
 * Kırılımlar (docs/tasarim/README.md): < 768 alt sekme çubuğu ve tek sütun;
 * 768–1279 ikon rayı ve iki panel (liste + ayrıntı); ≥ 1280 çok sütun.
 * Saf modül, node --test ile sınanır.
 */
export type Yerlesim = 'telefon' | 'tablet' | 'genis';

export function yerlesim(width: number): Yerlesim {
  if (width >= 1280) return 'genis';
  if (width >= 768) return 'tablet';
  return 'telefon';
}

/** Ray mı alt çubuk mu: tablet ve geniş ekranda ray. */
export const rayMi = (width: number) => yerlesim(width) !== 'telefon';

/** Dokunma hedefi en az 44 px (erişilebilirlik). */
export const DOKUNMA = 44;

/** Rozet metni: 0 → null, 100+ → "99+". */
export function rozet(n: number | null | undefined): string | null {
  if (!n || n < 0) return null;
  return n > 99 ? '99+' : String(n);
}

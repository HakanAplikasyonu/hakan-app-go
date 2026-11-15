/** Mükellef (şirket) membership of the signed-in user (`Me.taxpayers`). Pure: tested with node --test. */
export type TaxpayerOption = {
  uuid: string;
  kind: 'legal' | 'individual';
  title: string;
  role: 'owner' | 'employee';
};

/**
 * Which company to open without asking: the one used last time if the user
 * still belongs to it, else the only one. Null → show the picker.
 */
export function pickTaxpayer(options: readonly TaxpayerOption[], lastUsed: string | null): string | null {
  if (lastUsed && options.some((o) => o.uuid === lastUsed)) return lastUsed;
  if (options.length === 1) return options[0].uuid;
  return null;
}

/** Picker order: alphabetical by title in Turkish collation. */
export function sortTaxpayers(options: readonly TaxpayerOption[]): TaxpayerOption[] {
  return [...options].sort((a, b) => a.title.localeCompare(b.title, 'tr'));
}

export const roleLabel = (role: TaxpayerOption['role']) => (role === 'owner' ? 'Sahip' : 'Çalışan');
export const kindLabel = (kind: TaxpayerOption['kind']) => (kind === 'legal' ? 'Tüzel kişi' : 'Şahıs');

/** Initials for the company avatar ("Yıldız Market Ltd. Şti." → "YM"). */
export function initials(title: string): string {
  const skip = new Set(['ltd', 'şti', 'a.ş', 'aş', 've', 'san', 'tic']);
  const words = title
    .split(/\s+/)
    .map((w) => w.replace(/[^\p{L}\p{N}]/gu, ''))
    .filter((w) => w && !skip.has(w.toLocaleLowerCase('tr')));
  const letters = words.slice(0, 2).map((w) => w[0].toLocaleUpperCase('tr'));
  return letters.join('') || '?';
}

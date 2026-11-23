/**
 * Türkiye IBAN'ı (#780): müşavir mükellefe banka hesabı eklerken IBAN
 * kontrol hanesi (mod 97) ekranda denetlenir; banka, IBAN'ın 5–9.
 * hanelerindeki EFT kodundan seçilir. Sunucu aynı denetimi yapar.
 */

/** Ekstre içe aktarmanın tanıdığı on büyük banka, listede gösterilme sırası. */
export const BANKS = [
  "ziraat",
  "halkbank",
  "vakifbank",
  "isbank",
  "garanti",
  "akbank",
  "yapikredi",
  "qnb",
  "denizbank",
  "kuveyt_turk",
] as const;

export type BankId = (typeof BANKS)[number];

/** IBAN'daki banka kodu (5–9. haneler) → banka. */
const BANK_BY_CODE: Record<string, BankId> = {
  "00010": "ziraat",
  "00012": "halkbank",
  "00015": "vakifbank",
  "00064": "isbank",
  "00062": "garanti",
  "00046": "akbank",
  "00067": "yapikredi",
  "00111": "qnb",
  "00134": "denizbank",
  "00205": "kuveyt_turk",
};

/** Boşlukları atar, büyük harfe çevirir. */
export function compactIban(raw: string): string {
  return raw.replace(/\s+/g, "").toUpperCase();
}

/** TR + 24 hane ve doğru kontrol hanesi. */
export function isValidTrIban(raw: string): boolean {
  const v = compactIban(raw);
  if (!/^TR\d{24}$/.test(v)) return false;
  const moved = v.slice(4) + v.slice(0, 4);
  let digits = "";
  for (const ch of moved)
    digits += /[A-Z]/.test(ch) ? String(ch.charCodeAt(0) - 55) : ch;
  // Uzun sayıyı parça parça mod 97.
  let rest = 0;
  for (let i = 0; i < digits.length; i += 7)
    rest = Number(String(rest) + digits.slice(i, i + 7)) % 97;
  return rest === 1;
}

/** IBAN'ın bankası; listede yoksa null. */
export function bankFromIban(raw: string): BankId | null {
  const v = compactIban(raw);
  if (!/^TR\d{7}/.test(v)) return null;
  return BANK_BY_CODE[v.slice(4, 9)] ?? null;
}

/** Dörtlü gruplar: TR12 0006 2000 ... */
export function formatIban(raw: string): string {
  return compactIban(raw)
    .replace(/(.{4})/g, "$1 ")
    .trim();
}

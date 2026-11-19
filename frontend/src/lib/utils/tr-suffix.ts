/**
 * Türkçe iyelik eki (3. tekil) rakamla yazılmış sayıya: "2'si", "3'ü",
 * "%90'ı", "100'ü". Ek, sayının okunuşunun son kelimesine göre seçilir
 * (sıfır, iki, on, doksan, yüz, bin, milyon …).
 */
const UNITS = ["ı", "i", "si", "ü", "ü", "i", "sı", "si", "i", "u"];
const TENS = ["", "u", "si", "u", "ı", "si", "ı", "i", "i", "ı"];

function ending(n: number): string {
  n = Math.floor(Math.abs(n));
  if (n === 0) return UNITS[0]!;
  if (n % 10 !== 0) return UNITS[n % 10]!;
  if (n % 100 !== 0) return TENS[(n % 100) / 10]!;
  if (n % 1000 !== 0) return "ü"; // yüz
  if (n % 1_000_000 !== 0) return "i"; // bin
  if (n % 1_000_000_000 !== 0) return "u"; // milyon
  return "ı"; // milyar
}

/** Sayı ve eki birlikte: `trPossessive(90)` → "90'ı". */
export function trPossessive(n: number): string {
  return `${n}'${ending(n)}`;
}

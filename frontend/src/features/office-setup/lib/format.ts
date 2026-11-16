/**
 * Turkish possessive suffix after a digit ("2'si", "3'ü", "6'sı"): the
 * suffix follows the number as it is read aloud. Covers 0–9 (the wizard
 * has six steps); larger numbers use the last digit, which is right for
 * every ending except whole tens.
 */
const TR_POSSESSIVE: Record<number, string> = {
  0: "'ı",
  1: "'i",
  2: "'si",
  3: "'ü",
  4: "'ü",
  5: "'i",
  6: "'sı",
  7: "'si",
  8: "'i",
  9: "'u",
};

export function trPossessive(n: number) {
  return `${n}${TR_POSSESSIVE[Math.abs(n) % 10]}`;
}

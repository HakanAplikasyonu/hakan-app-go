/**
 * Ondalık string tutarlar için kuruş tabanlı yardımcılar. Float yok:
 * "1284560.40" → 128456040 kuruş (bigint). UBL tutarları en fazla 2 haneye yuvarlanır.
 */

export function toMinor(value: string | null | undefined, scale = 2): bigint {
  if (!value) return BigInt(0);
  const s = value.trim();
  const neg = s.startsWith("-");
  const [int = "0", frac = ""] = s.replace(/^[-+]/, "").split(".");
  const padded = (frac + "0".repeat(scale + 1)).slice(0, scale + 1);
  let minor =
    BigInt(int || "0") * BigInt(10) ** BigInt(scale) +
    BigInt(padded.slice(0, scale) || "0");
  // yarım yukarı yuvarlama
  if (Number(padded[scale] ?? "0") >= 5) minor += BigInt(1);
  return neg ? -minor : minor;
}

export function fromMinor(minor: bigint, scale = 2): string {
  const neg = minor < BigInt(0);
  const abs = neg ? -minor : minor;
  const base = BigInt(10) ** BigInt(scale);
  const int = abs / base;
  const frac = (abs % base).toString().padStart(scale, "0");
  return `${neg ? "-" : ""}${int}.${frac}`;
}

export function sum(values: (string | null | undefined)[]): string {
  return fromMinor(
    values.reduce<bigint>((acc, v) => acc + toMinor(v), BigInt(0)),
  );
}

export function absDiff(a: string, b: string): bigint {
  const d = toMinor(a) - toMinor(b);
  return d < BigInt(0) ? -d : d;
}

/** "1284560.40" → "1.284.560,40" (Türkçe gruplama, her zaman 2 hane). */
export function formatAmount(value: string | null | undefined): string {
  const s = fromMinor(toMinor(value));
  const neg = s.startsWith("-");
  const [int, frac] = s.replace("-", "").split(".");
  const grouped = int.replace(/\B(?=(\d{3})+(?!\d))/g, ".");
  return `${neg ? "-" : ""}${grouped},${frac}`;
}

const symbols: Record<string, string> = {
  TRY: "₺",
  USD: "$",
  EUR: "€",
  GBP: "£",
};

export function formatMoney(
  value: string | null | undefined,
  currency = "TRY",
) {
  const sym = symbols[currency];
  return sym
    ? `${sym} ${formatAmount(value)}`
    : `${formatAmount(value)} ${currency}`;
}

/** a × b, 2 haneye yarım yukarı yuvarlanmış (girdiler en fazla 4 hane). */
export function mul(a: string, b: string): string {
  const p = toMinor(a, 4) * toMinor(b, 4); // ölçek 8
  const half = BigInt(5) * BigInt(10) ** BigInt(5);
  const q = (p + (p < BigInt(0) ? -half : half)) / BigInt(10) ** BigInt(6);
  return fromMinor(q);
}

/** taban × yüzde / 100, 2 haneye yarım yukarı yuvarlanmış. */
export function percentOf(base: string, percent: string): string {
  const p = toMinor(base, 2) * toMinor(percent, 4); // ölçek 6, ×100
  const div = BigInt(100) * BigInt(10) ** BigInt(4);
  const half = div / BigInt(2);
  return fromMinor((p + (p < BigInt(0) ? -half : half)) / div);
}

export function add(...values: string[]): string {
  return sum(values);
}

export function sub(a: string, b: string): string {
  return fromMinor(toMinor(a) - toMinor(b));
}

/**
 * Elle yazılan tutarı API biçimine çevirir ("1.234,56" → "1234.56").
 * Türkçe yazım esas: virgül ondalık, nokta binlik ayırıcıdır. Yalnız nokta
 * varsa ve sonrasında tam üç hane kalıyorsa ("1.234") binlik sayılır; diğer
 * tek nokta ("12.5") ondalıktır. ₺ ve boşluk atılır. Boş girdi "", sayı
 * olmayan girdi null döner.
 */
export function parseAmountInput(input: string): string | null {
  const s = input.replace(/[\s₺]/g, "");
  if (!s) return "";
  if (!/^[0-9.,]+$/.test(s)) return null;
  let norm: string;
  if (s.includes(",")) {
    if (s.indexOf(",") !== s.lastIndexOf(",")) return null;
    const [int, frac] = s.split(",");
    if (int.includes(".") && !/^\d{1,3}(\.\d{3})+$/.test(int)) return null;
    norm = `${int.replace(/\./g, "")}.${frac}`;
  } else {
    const dots = s.split(".").length - 1;
    if (dots > 1 || /^\d{1,3}\.\d{3}$/.test(s)) {
      if (!/^\d{1,3}(\.\d{3})+$/.test(s)) return null;
      norm = s.replace(/\./g, "");
    } else {
      norm = s;
    }
  }
  if (!/^\d*(\.\d{0,2})?$/.test(norm) || norm === ".") return null;
  return fromMinor(toMinor(norm.startsWith(".") ? `0${norm}` : norm));
}

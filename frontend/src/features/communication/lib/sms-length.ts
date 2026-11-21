/**
 * SMS boyu sayacı: sunucudaki communication.SMSLength kurallarının aynısı
 * (#780). TR kodlamasında ç ğ ı ş Ğ İ Ş ve ^ { } [ ] ~ | \ iki karakter
 * sayılır; tek SMS 155, çok parçalı SMS'in her parçası 148 karakterdir;
 * operatör sona 5 karakterlik ek koyar. UNICODE'da tek SMS 70, parça 67.
 */
export const SINGLE_SMS_CHARS = 155;
export const MULTI_SMS_CHARS = 148;
export const OPERATOR_SUFFIX = 5;
/** Toplu gönderimde kişiselleştirilmiş SMS en fazla bu kadar parça olabilir. */
export const BULK_SMS_PART_LIMIT = 3;

const DOUBLE = new Set([
  "ç",
  "ğ",
  "ı",
  "ş",
  "Ğ",
  "İ",
  "Ş",
  "^",
  "{",
  "}",
  "[",
  "]",
  "~",
  "|",
  "\\",
]);

/** Sunucunun NormalizeText'i: para işaretleri çevrilir, baş/son boşluk atılır. */
export function normalizeSmsText(text: string): string {
  return text
    .replace(/\r\n/g, "\n")
    .replace(/₺/g, "TL")
    .replace(/€/g, "EUR")
    .replace(/£/g, "GBP")
    .trim();
}

export type SmsLength = { chars: number; parts: number };

export function smsLength(text: string, encoding = "TR"): SmsLength {
  const chars = Array.from(normalizeSmsText(text));
  if (encoding === "UNICODE") {
    const n = chars.length;
    return { chars: n, parts: n > 70 ? Math.floor((n + 66) / 67) : 1 };
  }
  let n = OPERATOR_SUFFIX;
  for (const c of chars) n += DOUBLE.has(c) ? 2 : 1;
  const parts = n > SINGLE_SMS_CHARS ? Math.ceil(n / MULTI_SMS_CHARS) : 1;
  return { chars: n, parts };
}

/** Metne eklenebilen değişkenler (sunucudaki Personalize ile aynı). */
export const MESSAGE_VARIABLES = [
  ":mukellef",
  ":ofis",
  ":donem",
  ":tutar",
  ":son_gun",
  ":link",
] as const;

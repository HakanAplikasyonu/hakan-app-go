/**
 * SMS uzunluğu (#780, NetGSM TR kodlaması): sunucudaki
 * communication.SMSLength'in aynısı. ç ğ ı ş Ğ İ Ş (ve ^ { } [ ] ~ | \)
 * iki karakter sayılır; operatör eki 5 karakter; tek SMS 155, çok parçalı
 * SMS'in her parçası 148 karakter.
 */
export const SINGLE_SMS_CHARS = 155;
export const MULTI_SMS_CHARS = 148;
export const OPERATOR_SUFFIX = 5;

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

export type SmsLength = { chars: number; parts: number };

export function smsLength(text: string): SmsLength {
  const trimmed = text.replace(/\n+$/, "");
  let chars = OPERATOR_SUFFIX;
  for (const ch of trimmed) chars += DOUBLE.has(ch) ? 2 : 1;
  const parts =
    chars > SINGLE_SMS_CHARS ? Math.ceil(chars / MULTI_SMS_CHARS) : 1;
  return { chars, parts };
}

/** Metin değişkenleri; etikete tıklanınca metne eklenir. */
export const AUTOMATION_VARIABLES = [
  "mukellef",
  "ofis",
  "donem",
  "tutar",
  "son_gun",
  "beyan",
  "banka",
  "link",
] as const;

export type AutomationVariable = (typeof AUTOMATION_VARIABLES)[number];

/** Önizlemenin örnek mükellefi (sunucudaki SampleVars ile aynı). */
export const SAMPLE_VARS: Record<AutomationVariable, string> = {
  mukellef: "Örnek Gıda Ticaret Ltd. Şti.",
  ofis: "Demo Mali Müşavirlik",
  donem: "Ekim 2026",
  tutar: "12.450,00 TL",
  son_gun: "28 Kasım 2026",
  beyan: "KDV",
  banka: "Ziraat Bankası",
  link: "https://app.ornek.com/m/abc",
};

const VAR_PATTERN =
  /:(mukellef|ofis|donem|tutar|son_gun|beyan|banka|link)(?![\p{L}\p{N}_])/gu;

/** Değişkenleri yerleştirir (sunucudaki Render ile aynı kurallar). */
export function renderVariables(
  text: string,
  vars: Partial<Record<AutomationVariable, string>>,
): string {
  let out = text.replace(
    VAR_PATTERN,
    (_, name: AutomationVariable) => vars[name] ?? "",
  );
  while (out.includes("  ")) out = out.replaceAll("  ", " ");
  out = out.replaceAll(" ,", ",").replaceAll(" .", ".");
  return out.trim();
}

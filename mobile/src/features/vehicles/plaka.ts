/**
 * Plaka biçimi (pure), sunucunun extract.NormalizePlate kuralıyla aynı:
 * boşluk, tire ve nokta atılır, büyük harfe çevrilir, Türkçe harfler
 * sadeleşir (İ→I, Ş→S …). Geçerli: il kodu 01–81, 1–3 harf, 2–5 rakam.
 */
const plateRe = /^(0[1-9]|[1-7][0-9]|8[01])[A-Z]{1,3}[0-9]{2,5}$/;

export function normalizePlate(s: string): string {
  return s
    .replace(/i/g, 'İ')
    .replace(/ı/g, 'I')
    .toLocaleUpperCase('tr')
    .replace(/İ/g, 'I')
    .replace(/Ş/g, 'S')
    .replace(/Ç/g, 'C')
    .replace(/Ğ/g, 'G')
    .replace(/Ö/g, 'O')
    .replace(/Ü/g, 'U')
    .replace(/[\s.-]/g, '');
}

export function plateValid(s: string): boolean {
  return plateRe.test(normalizePlate(s));
}

/** 34ABC123 → "34 ABC 123" (okuma kolaylığı). */
export function formatPlate(p: string): string {
  const m = /^(\d{2})([A-Z]{1,3})(\d+)$/.exec(p);
  return m ? `${m[1]} ${m[2]} ${m[3]}` : p;
}

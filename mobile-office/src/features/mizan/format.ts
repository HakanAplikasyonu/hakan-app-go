/** Mizan sekmesi yardımcıları (#393): saf modül, node --test ile sınanır. */

const MONTHS = ['Ocak', 'Şubat', 'Mart', 'Nisan', 'Mayıs', 'Haziran', 'Temmuz', 'Ağustos', 'Eylül', 'Ekim', 'Kasım', 'Aralık'];

function num(s: string): number {
  const n = Number(s);
  return Number.isFinite(n) ? n : 0;
}

function tr(n: number): string {
  return Math.abs(n).toLocaleString('tr-TR', { minimumFractionDigits: 2, maximumFractionDigits: 2 });
}

/** Mizanın adı: dönem ("Ekim 2026") ya da yükleme tarihi. */
export function trialBalanceLabel(t: { period: string | null; created_at: string }): string {
  const m = t.period ? /^(\d{4})-(\d{2})$/.exec(t.period) : null;
  if (m) return `${MONTHS[Number(m[2]) - 1]} ${m[1]}`;
  return `${new Date(t.created_at).toLocaleDateString('tr-TR', { timeZone: 'Europe/Istanbul' })} yüklemesi`;
}

/** İşaretli bakiye → "7.150,00 ₺ B" (borç) / "35.500,50 ₺ A" (alacak) / "0,00 ₺". */
export function balanceLabel(balance: string): string {
  const n = num(balance);
  if (n === 0) return '0,00 ₺';
  return `${tr(n)} ₺ ${n > 0 ? 'B' : 'A'}`;
}

/** Ekran okuyucu için: "borç bakiyesi 7.150,00 lira". */
export function balanceSpoken(balance: string): string {
  const n = num(balance);
  if (n === 0) return 'bakiye yok';
  return `${n > 0 ? 'borç' : 'alacak'} bakiyesi ${tr(n)} lira`;
}

export type ChangeTone = 'up' | 'down' | 'flat' | 'new';

/**
 * Önceki döneme göre değişim: tutar (işaretli, "+400,00 ₺"), yüzde (önceki
 * bakiye sıfır değilse) ve yön. Önceki dosyada olmayan hesap "new".
 */
export function changeOf(l: { balance: string; previous_balance: string | null; change: string | null }): {
  text: string;
  percent: string | null;
  tone: ChangeTone;
} | null {
  if (l.change === null) return null;
  const c = num(l.change);
  if (l.previous_balance === null) return { text: 'Yeni hesap', percent: null, tone: 'new' };
  if (c === 0) return { text: 'Değişmedi', percent: null, tone: 'flat' };
  const prev = num(l.previous_balance);
  const percent = prev !== 0 ? `%${Math.abs((c / Math.abs(prev)) * 100).toLocaleString('tr-TR', { maximumFractionDigits: 1 })}` : null;
  return { text: `${c > 0 ? '+' : '−'}${tr(c)} ₺`, percent, tone: c > 0 ? 'up' : 'down' };
}

/** Arama: kod öneki ya da adın içinde (tr-TR küçük harf). */
export function filterLines<T extends { code: string; name: string }>(lines: readonly T[], q: string): T[] {
  const t = q.trim().toLocaleLowerCase('tr');
  if (!t) return [...lines];
  return lines.filter((l) => l.code.startsWith(t) || l.name.toLocaleLowerCase('tr').includes(t));
}

/** Borç ve alacak toplamı tutuyor mu (kuruş farkı yok sayılır). */
export function isBalanced(t: { debit_total: string; credit_total: string }): boolean {
  return Math.abs(num(t.debit_total) - num(t.credit_total)) < 0.005;
}

/** Paylaşılan metin: başlık, toplamlar ve ana hesap bakiyeleri. */
export function shareText(
  title: string,
  label: string,
  t: { debit_total: string; credit_total: string },
  lines: readonly { code: string; name: string; level: number; balance: string }[],
  comparedTo: string | null,
): string {
  const head = [`${title} · ${label} mizanı`, `Borç toplamı: ${tr(num(t.debit_total))} ₺`, `Alacak toplamı: ${tr(num(t.credit_total))} ₺`];
  if (comparedTo) head.push(`Karşılaştırma: ${comparedTo}`);
  const rows = lines.filter((l) => l.level === 1).map((l) => `${l.code} ${l.name}: ${balanceLabel(l.balance)}`);
  return [...head, '', ...rows].join('\n');
}

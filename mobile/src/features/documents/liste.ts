/**
 * "Belgelerim" ekranının saf yardımcıları (#780): bölümler, etiketler,
 * geçerlilik metni. node --test ile sınanır.
 */

type ListeBelgesi = {
  uuid: string;
  category: string;
  important_kind: string | null;
  expiry_status: string;
  days_left: number | null;
};

const KATEGORI: Record<string, string> = {
  general: 'Genel',
  contract: 'Sözleşme',
  tax: 'Vergi',
  sgk: 'SGK',
  trade_registry: 'Ticaret sicil',
  isg: 'İSG',
  other: 'Diğer',
};

const ONEMLI: Record<string, string> = {
  tax_certificate: 'Vergi levhası',
  signature_circular: 'İmza sirküleri',
  trade_registry_gazette: 'Ticaret sicil gazetesi',
  lease_contract: 'Kira sözleşmesi',
  license: 'Ruhsat',
  power_of_attorney: 'Vekâletname',
  other: 'Diğer önemli belge',
};

/** Satırın türü: önemli belgede türü, değilse kategorisi. */
export function belgeEtiketi(d: Pick<ListeBelgesi, 'category' | 'important_kind'>): string {
  if (d.important_kind) return ONEMLI[d.important_kind] ?? 'Önemli belge';
  return KATEGORI[d.category] ?? 'Belge';
}

/** "12 gün kaldı" / "Süresi geçti (3 gün)"; geçerlilik tarihi yoksa boş. */
export function gecerlilikMetni(d: Pick<ListeBelgesi, 'expiry_status' | 'days_left'>): string {
  if (d.days_left === null) return '';
  if (d.expiry_status === 'expired') return `Süresi geçti (${Math.abs(d.days_left)} gün)`;
  if (d.expiry_status === 'expiring') return d.days_left === 0 ? 'Bugün bitiyor' : `${d.days_left} gün kaldı`;
  return '';
}

/**
 * Önemli belgeler üstte (sunucunun sırası: bitişi yakın önce), kalan
 * paylaşılan belgeler altta; aynı belge iki bölümde görünmez. Boş bölüm yok.
 */
export function belgeBolumleri<T extends { uuid: string }>(onemli: readonly T[], paylasilan: readonly T[]): { title: string; data: T[] }[] {
  const ust = new Set(onemli.map((d) => d.uuid));
  const alt = paylasilan.filter((d) => !ust.has(d.uuid));
  const out: { title: string; data: T[] }[] = [];
  if (onemli.length) out.push({ title: 'Önemli belgeler', data: [...onemli] });
  if (alt.length) out.push({ title: 'Paylaşılan belgeler', data: alt });
  return out;
}

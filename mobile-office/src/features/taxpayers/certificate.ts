/**
 * Vergi levhasından mükellef ekleme (#780): saf yardımcılar, node --test ile
 * sınanır. Okuma ucu yapay zekâyla levhayı okur; okunamayan alan boş kalır
 * ve formda "Kontrol et" ile işaretlenir.
 */

export type TaxpayerKind = 'legal' | 'individual';

/** Formun alanları; tümü düz metin. */
export type NewTaxpayerForm = {
  tax_id: string;
  title: string;
  contact_email: string;
  tax_office: string;
  address: string;
  activity_code: string;
};

/** Levhadan okunan alanlar (e-posta levhada yoktur). */
export type CertificateField = 'tax_id' | 'title' | 'tax_office' | 'address' | 'activity_code';

export const certificateFields: readonly CertificateField[] = ['tax_id', 'title', 'tax_office', 'address', 'activity_code'];

export const emptyForm: NewTaxpayerForm = { tax_id: '', title: '', contact_email: '', tax_office: '', address: '', activity_code: '' };

/** Okuma ucunun yanıtı; her alan boş ya da null olabilir. */
export type CertificateReading = Partial<Record<CertificateField, string | null>>;

/** VKN / TCKN: boşluk, nokta ve tire atılır (sunucu da böyle yapar). */
export function normalizeTaxId(s: string): string {
  return s.replace(/[\s.-]/g, '');
}

/** 10 hane VKN → tüzel, 11 hane TCKN → gerçek kişi; başka biçim null. */
export function taxIdKind(s: string): TaxpayerKind | null {
  const id = normalizeTaxId(s);
  if (/^\d{10}$/.test(id)) return 'legal';
  if (/^\d{11}$/.test(id)) return 'individual';
  return null;
}

/**
 * Okunanları forma yazar: okunan alan formdakinin yerine geçer, okunamayan
 * alan boşaltılır ve `unread` listesine girer. E-posta olduğu gibi kalır.
 */
export function applyReading(form: NewTaxpayerForm, r: CertificateReading): { form: NewTaxpayerForm; unread: CertificateField[] } {
  const next = { ...form };
  const unread: CertificateField[] = [];
  for (const f of certificateFields) {
    const raw = r[f];
    const v = typeof raw === 'string' ? raw.trim() : '';
    if (!v) {
      next[f] = '';
      unread.push(f);
      continue;
    }
    next[f] = f === 'tax_id' ? normalizeTaxId(v) : v;
  }
  return { form: next, unread };
}

const EMAIL = /^[^\s@]+@[^\s@]+\.[^\s@]+$/;

/** Kaydetmeden önce denetim; sunucunun zorunlu alanları: VKN/TCKN, unvan, e-posta. */
export function validateNewTaxpayer(f: NewTaxpayerForm): Partial<Record<keyof NewTaxpayerForm, string>> {
  const e: Partial<Record<keyof NewTaxpayerForm, string>> = {};
  if (!taxIdKind(f.tax_id)) e.tax_id = 'VKN 10, TCKN 11 haneli olmalı.';
  const title = f.title.trim();
  if (!title) e.title = 'Unvanı yazın.';
  else if (title.length > 255) e.title = 'Unvan en çok 255 karakter olabilir.';
  if (!EMAIL.test(f.contact_email.trim())) e.contact_email = 'Onay bağlantısının gideceği e-postayı yazın.';
  return e;
}

/** Ekle isteğinin gövdesi (`POST /v1/tenant/taxpayer-requests`). */
export function requestBody(f: NewTaxpayerForm) {
  const tax_id = normalizeTaxId(f.tax_id);
  return { kind: taxIdKind(tax_id) ?? 'legal', tax_id, title: f.title.trim(), contact_email: f.contact_email.trim() };
}

/**
 * Şirketin iletişim bilgisi (#780): ofisin mükellef için tuttuğu telefon ve
 * e-posta, `/v1/taxpayer/context` içinde. Mükellef yalnız görür; değişiklik
 * için müşavirine yazar. Saf modül, node --test ile sınanır.
 */

export type TaxpayerContact = { phone: string | null; email: string | null } | null | undefined;

export type ContactRow = { key: 'phone' | 'email'; label: string; value: string; set: boolean };

const EMPTY = 'Kayıtlı değil';

export function contactRows(contact: TaxpayerContact): ContactRow[] {
  const phone = contact?.phone?.trim() || '';
  const email = contact?.email?.trim() || '';
  return [
    { key: 'phone', label: 'Telefon', value: phone || EMPTY, set: !!phone },
    { key: 'email', label: 'E-posta', value: email || EMPTY, set: !!email },
  ];
}

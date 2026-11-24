/**
 * Mükellef iletişim bilgisi yardımcıları (#780): saf modül, node --test ile
 * sınanır. Telefonu ve e-postayı sunucu denetler ve düzeltir; burada yalnız
 * ekran metni üretilir.
 */

export type ContactMember = { name: string; surname: string; email: string; role: string };
export type ErrorDetailLike = { field?: string; code?: string };

/** Yetkili kişi: mükellef hesabının sahipleri ("Ayşe Demir, Ali Kaya"); yoksa null. */
export function contactPerson(members: ContactMember[] | undefined): string | null {
  const owners = (members ?? []).filter((m) => m.role === 'owner').map((m) => `${m.name} ${m.surname}`.trim());
  return owners.length ? owners.join(', ') : null;
}

/** Sunucunun alan hata kodları (`telefon_gecersiz`, `eposta_gecersiz`) → Türkçe ileti. */
export function contactFieldErrors(details: ErrorDetailLike[] | undefined): { phone?: string; email?: string } {
  const out: { phone?: string; email?: string } = {};
  for (const d of details ?? []) {
    if (d.code === 'telefon_gecersiz') out.phone = 'Telefon numarası geçersiz. Örnek: 0532 123 45 67';
    if (d.code === 'eposta_gecersiz') out.email = 'E-posta adresi geçersiz. Örnek: muhasebe@firma.com.tr';
  }
  return out;
}

/** Gönderilecek gövde: kırpılmış; boş alan bilgiyi siler. */
export function contactBody(phone: string, email: string): { phone: string; email: string } {
  return { phone: phone.trim(), email: email.trim() };
}
